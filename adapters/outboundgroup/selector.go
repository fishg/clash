package outboundgroup

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/Dreamacro/clash/adapters/outbound"
	"github.com/Dreamacro/clash/adapters/provider"
	"github.com/Dreamacro/clash/common/singledo"
	C "github.com/Dreamacro/clash/constant"
)

type Selector struct {
	*outbound.Base
	disableUDP bool
	single     *singledo.Single
	selected   string
	autoBackup bool
	providers  []provider.ProxyProvider
}

// DialContext implements C.ProxyAdapter
func (s *Selector) DialContext(ctx context.Context, metadata *C.Metadata) (C.Conn, error) {
	c, err := s.selectedProxy(true).DialContext(ctx, metadata)
	if err == nil {
		c.AppendToChains(s)
	}
	return c, err
}

// DialUDP implements C.ProxyAdapter
func (s *Selector) DialUDP(metadata *C.Metadata) (C.PacketConn, error) {
	pc, err := s.selectedProxy(true).DialUDP(metadata)
	if err == nil {
		pc.AppendToChains(s)
	}
	return pc, err
}

// SupportUDP implements C.ProxyAdapter
func (s *Selector) SupportUDP() bool {
	if s.disableUDP {
		return false
	}

	return s.selectedProxy(false).SupportUDP()
}

// MarshalJSON implements C.ProxyAdapter
func (s *Selector) MarshalJSON() ([]byte, error) {
	var all []string
	for _, proxy := range getProvidersProxies(s.providers, false) {
		all = append(all, proxy.Name())
	}

	return json.Marshal(map[string]interface{}{
		"type": s.Type().String(),
		"now":  s.Now(),
		"all":  all,
	})
}

func (s *Selector) Now() string {
	return s.selectedProxy(false).Name()
}

func (s *Selector) Set(name string) error {
	for _, proxy := range getProvidersProxies(s.providers, false) {
		if proxy.Name() == name {
			s.selected = name
			s.single.Reset()
			return nil
		}
	}

	return errors.New("proxy not exist")
}

// Unwrap implements C.ProxyAdapter
func (s *Selector) Unwrap(metadata *C.Metadata) C.Proxy {
	return s.selectedProxy(true)
}

func (s *Selector) selectedProxy(touch bool) C.Proxy {
	var groupTypes = map[string]string{"Direct": "1", "Reject": "1", "Pass": "1", "Relay": "1", "Selector": "1", "Fallback": "1", "URLTest": "1", "LoadBalance": "1"}
	elm, _, _ := s.single.Do(func() (interface{}, error) {
		proxies := getProvidersProxies(s.providers, touch)
		for _, proxy := range proxies {
			if proxy.Name() == s.selected {
				_, groupType := groupTypes[proxy.Type().String()]
				if groupType || proxy.Alive() || !s.autoBackup {
					return proxy, nil
				}
			}
		}
		fast := proxies[0]
		if s.autoBackup {
			//if autoBackup , choose min delay node
			min := fast.LastDelay()
			for _, proxy := range proxies[1:] {
				if !proxy.Alive() {
					continue
				}
				delay := proxy.LastDelay()
				if delay < min {
					fast = proxy
					min = delay
				}
			}
		}

		return fast, nil
	})

	return elm.(C.Proxy)
}

func NewSelector(options *GroupCommonOption, providers []provider.ProxyProvider) *Selector {
	selected := providers[0].Proxies()[0].Name()
	return &Selector{
		Base:       outbound.NewBase(options.Name, "", "", C.Selector, false, 0, 0, 0, 1),
		single:     singledo.NewSingle(defaultGetProxiesDuration),
		providers:  providers,
		selected:   selected,
		disableUDP: options.DisableUDP,
		autoBackup: options.AutoBackup,
	}
}
