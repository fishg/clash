package geosite

import (
	"errors"
	"io/ioutil"
	"os"
	"runtime"
	"strings"

	C "github.com/Dreamacro/clash/constant"
	"google.golang.org/protobuf/proto"
)

/*func loadGeoIP(code string) ([]*CIDR, error) {
	return loadIP("geoip.dat", code)
}*/

var (
	FileCache = make(map[string][]byte)
	//IPCache   = make(map[string]*GeoIP)
	SiteCache = make(map[string]*GeoSite)
)

const (
	errCodeTruncated = -1
	errCodeOverflow  = -3
)

func ReadFile(path string) ([]byte, error) {
	reader, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return ioutil.ReadAll(reader)
}

func ReadAsset(file string) ([]byte, error) {
	return ReadFile(file)
}

func loadFile(file string) ([]byte, error) {
	if FileCache[file] == nil {
		bs, err := ReadAsset(file)
		if err != nil {
			return nil, errors.New("failed to open file: " + file)
		}
		if len(bs) == 0 {
			return nil, errors.New("empty file: " + file)
		}
		// Do not cache file, may save RAM when there
		// are many files, but consume CPU each time.
		return bs, nil
		//FileCache[file] = bs
	}
	return FileCache[file], nil
}

/*func loadIP(file, code string) ([]*CIDR, error) {
	index := file + ":" + code
	if IPCache[index] == nil {
		bs, err := loadFile(file)
		if err != nil {
			return nil, errors.New("failed to load file: " + file)
		}
		bs = find(bs, []byte(code))
		if bs == nil {
			return nil, errors.New("code not found in " + file + ": " + code)
		}
		var geoip GeoIP
		if err := proto.Unmarshal(bs, &geoip); err != nil {
			return nil, errors.New("error unmarshal IP in " + file + ": " + code)
		}
		defer runtime.GC()     // or debug.FreeOSMemory()
		//IPCache[index] = &geoip
		return geoip.Cidr, nil // do not cache geoip
		//IPCache[index] = &geoip
	}
	return IPCache[index].Cidr, nil
}*/

func loadSite(file, code string) ([]*Domain, error) {
	index := file + ":" + code
	if SiteCache[index] == nil {
		bs, err := loadFile(C.Path.GeoSite())
		if err != nil {
			return nil, errors.New("failed to load file: " + file)
		}
		bs = find(bs, []byte(code))
		if bs == nil {
			return nil, errors.New("list not found in " + file + ": " + code)
		}
		var geosite GeoSite
		if err := proto.Unmarshal(bs, &geosite); err != nil {
			return nil, errors.New("error unmarshal Site in " + file + ": " + code)
		}
		defer runtime.GC() // or debug.FreeOSMemory()
		//SiteCache[index] = &geosite
		return geosite.Domain, nil // do not cache geosite
		//SiteCache[index] = &geosite
	}
	return SiteCache[index].Domain, nil
}

func find(data, code []byte) []byte {
	codeL := len(code)
	if codeL == 0 {
		return nil
	}
	for {
		dataL := len(data)
		if dataL < 2 {
			return nil
		}
		x, y := decodeVarint(data[1:])
		if x == 0 && y == 0 {
			return nil
		}
		headL, bodyL := 1+y, int(x)
		dataL -= headL
		if dataL < bodyL {
			return nil
		}
		data = data[headL:]
		if int(data[1]) == codeL {
			for i := 0; i < codeL && data[2+i] == code[i]; i++ {
				if i+1 == codeL {
					return data[:bodyL]
				}
			}
		}
		if dataL == bodyL {
			return nil
		}
		data = data[bodyL:]
	}
}

type AttributeMatcher interface {
	Match(*Domain) bool
}

type BooleanMatcher string

func (m BooleanMatcher) Match(domain *Domain) bool {
	for _, attr := range domain.Attribute {
		if attr.Key == string(m) {
			return true
		}
	}
	return false
}

type AttributeList struct {
	matcher []AttributeMatcher
}

func (al *AttributeList) Match(domain *Domain) bool {
	for _, matcher := range al.matcher {
		if !matcher.Match(domain) {
			return false
		}
	}
	return true
}

func (al *AttributeList) IsEmpty() bool {
	return len(al.matcher) == 0
}

func parseAttrs(attrs []string) *AttributeList {
	al := new(AttributeList)
	for _, attr := range attrs {
		lc := strings.ToLower(attr)
		al.matcher = append(al.matcher, BooleanMatcher(lc))
	}
	return al
}

func loadGeositeWithAttr(file string, siteWithAttr string) ([]*Domain, error) {
	parts := strings.Split(siteWithAttr, "@")
	if len(parts) == 0 {
		return nil, errors.New("empty site")
	}
	country := strings.ToUpper(parts[0])
	attrs := parseAttrs(parts[1:])
	domains, err := loadSite(file, country)
	if err != nil {
		return nil, err
	}

	if attrs.IsEmpty() {
		return domains, nil
	}

	filteredDomains := make([]*Domain, 0, len(domains))
	for _, domain := range domains {
		if attrs.Match(domain) {
			filteredDomains = append(filteredDomains, domain)
		}
	}

	return filteredDomains, nil
}

func consumeVarint(b []byte) (v uint64, n int) {
	var y uint64
	if len(b) <= 0 {
		return 0, errCodeTruncated
	}
	v = uint64(b[0])
	if v < 0x80 {
		return v, 1
	}
	v -= 0x80

	if len(b) <= 1 {
		return 0, errCodeTruncated
	}
	y = uint64(b[1])
	v += y << 7
	if y < 0x80 {
		return v, 2
	}
	v -= 0x80 << 7

	if len(b) <= 2 {
		return 0, errCodeTruncated
	}
	y = uint64(b[2])
	v += y << 14
	if y < 0x80 {
		return v, 3
	}
	v -= 0x80 << 14

	if len(b) <= 3 {
		return 0, errCodeTruncated
	}
	y = uint64(b[3])
	v += y << 21
	if y < 0x80 {
		return v, 4
	}
	v -= 0x80 << 21

	if len(b) <= 4 {
		return 0, errCodeTruncated
	}
	y = uint64(b[4])
	v += y << 28
	if y < 0x80 {
		return v, 5
	}
	v -= 0x80 << 28

	if len(b) <= 5 {
		return 0, errCodeTruncated
	}
	y = uint64(b[5])
	v += y << 35
	if y < 0x80 {
		return v, 6
	}
	v -= 0x80 << 35

	if len(b) <= 6 {
		return 0, errCodeTruncated
	}
	y = uint64(b[6])
	v += y << 42
	if y < 0x80 {
		return v, 7
	}
	v -= 0x80 << 42

	if len(b) <= 7 {
		return 0, errCodeTruncated
	}
	y = uint64(b[7])
	v += y << 49
	if y < 0x80 {
		return v, 8
	}
	v -= 0x80 << 49

	if len(b) <= 8 {
		return 0, errCodeTruncated
	}
	y = uint64(b[8])
	v += y << 56
	if y < 0x80 {
		return v, 9
	}
	v -= 0x80 << 56

	if len(b) <= 9 {
		return 0, errCodeTruncated
	}
	y = uint64(b[9])
	v += y << 63
	if y < 2 {
		return v, 10
	}
	return 0, errCodeOverflow
}

func decodeVarint(b []byte) (uint64, int) {
	v, n := consumeVarint(b)
	if n < 0 {
		return 0, 0
	}
	return v, n
}