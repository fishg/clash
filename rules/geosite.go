package rules

import (
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	
	"github.com/Dreamacro/clash/common/buf"
	C "github.com/Dreamacro/clash/constant"
	"github.com/Dreamacro/clash/log"
	"github.com/golang/protobuf/proto"
)

type GEOSITE struct {
	country     string
	adapter     string
	noResolveIP bool
}

var (
	FileCache = make(map[string][]byte)
	//IPCache   = make(map[string]*GeoIP)
	SiteCache = make(map[string]*GeoSite)
)

type FileReaderFunc func(path string) (io.ReadCloser, error)

var NewFileReader FileReaderFunc = func(path string) (io.ReadCloser, error) {
	return os.Open(path)
}

func ReadFile(path string) ([]byte, error) {
	reader, err := NewFileReader(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return buf.ReadAllToBytes(reader)
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
		FileCache[file] = bs
	}
	return FileCache[file], nil
}

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
		defer runtime.GC()         // or debug.FreeOSMemory()
		return geosite.Domain, nil // do not cache geosite
		SiteCache[index] = &geosite
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
		x, y := proto.DecodeVarint(data[1:])
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

func (g *GEOSITE) RuleType() C.RuleType {
	return C.GEOSITE
}

func (g *GEOSITE) Match(metadata *C.Metadata) bool {
	if metadata.AddrType != C.AtypDomainName {
		return false
	}
	
	domain := metadata.Host
	country := g.country
	
	domains, err := loadGeositeWithAttr("geosite.dat", country)
	if err != nil {
		//log.Infoln("HTTP proxy listening at: %s", addr)
		log.Errorln("failed to load geosite: %s, base error: %s", country, err.Error())
		return false
	}
	
	matcher, err := NewDomainMatcher(domains)
	
	if err != nil {
		log.Errorln("failed to new DomainMatcher: %s", err.Error())
		return false
	}
	
	r := matcher.ApplyDomain(domain)
	
	return r
}

func (g *GEOSITE) Adapter() string {
	return g.adapter
}

func (g *GEOSITE) Payload() string {
	return g.country
}

func (g *GEOSITE) ShouldResolveIP() bool {
	return !g.noResolveIP
}

func NewGEOSITE(country string, adapter string, noResolveIP bool) *GEOSITE {
	geosite := &GEOSITE{
		country:     country,
		adapter:     adapter,
		noResolveIP: noResolveIP,
	}

	return geosite
}
