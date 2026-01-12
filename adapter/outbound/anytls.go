package outbound

import (
	"context"
	"net"
	"strconv"
	"sync"
	"time"

	N "github.com/metacubex/mihomo/common/net"
	"github.com/metacubex/mihomo/component/proxydialer"
	"github.com/metacubex/mihomo/config"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/transport/anytls"
	"github.com/metacubex/mihomo/transport/vmess"

	M "github.com/metacubex/sing/common/metadata"
	"github.com/metacubex/sing/common/uot"
)

// Global session management config for AnyTLS
var (
	globalAnyTLSSessionConfig     *config.AnyTLSSessionManagement
	globalAnyTLSSessionConfigLock sync.RWMutex
)

// SetGlobalAnyTLSSessionConfig sets the global session management configuration
func SetGlobalAnyTLSSessionConfig(cfg *config.AnyTLSSessionManagement) {
	globalAnyTLSSessionConfigLock.Lock()
	defer globalAnyTLSSessionConfigLock.Unlock()
	globalAnyTLSSessionConfig = cfg
}

// getGlobalAnyTLSSessionConfig retrieves the global session management configuration
func getGlobalAnyTLSSessionConfig() *config.AnyTLSSessionManagement {
	globalAnyTLSSessionConfigLock.RLock()
	defer globalAnyTLSSessionConfigLock.RUnlock()
	return globalAnyTLSSessionConfig
}

type AnyTLS struct {
	*Base
	client *anytls.Client
	option *AnyTLSOption
}

type AnyTLSOption struct {
	BasicOption
	Name                        string                  `proxy:"name"`
	Server                      string                  `proxy:"server"`
	Port                        int                     `proxy:"port"`
	Password                    string                  `proxy:"password"`
	ALPN                        []string                `proxy:"alpn,omitempty"`
	SNI                         string                  `proxy:"sni,omitempty"`
	ECHOpts                     ECHOptions              `proxy:"ech-opts,omitempty"`
	ClientFingerprint           string                  `proxy:"client-fingerprint,omitempty"`
	SkipCertVerify              bool                    `proxy:"skip-cert-verify,omitempty"`
	Fingerprint                 string                  `proxy:"fingerprint,omitempty"`
	Certificate                 string                  `proxy:"certificate,omitempty"`
	PrivateKey                  string                  `proxy:"private-key,omitempty"`
	UDP                         bool                    `proxy:"udp,omitempty"`
	IdleSessionCheckInterval    int                     `proxy:"idle-session-check-interval,omitempty"`
	IdleSessionTimeout          int                     `proxy:"idle-session-timeout,omitempty"`
	MinIdleSession              int                     `proxy:"min-idle-session,omitempty"`
	SessionOverride             *AnyTLSSessionOverride  `proxy:"session-override,omitempty"` // Per-proxy overrides
}

// AnyTLSSessionOverride allows per-proxy overrides of global session management settings
type AnyTLSSessionOverride struct {
	EnsureIdleSession           *int `proxy:"ensure-idle-session,omitempty"`
	MinIdleSession              *int `proxy:"min-idle-session,omitempty"`
	MinIdleSessionForAge        *int `proxy:"min-idle-session-for-age,omitempty"`
	EnsureIdleSessionCreateRate *int `proxy:"ensure-idle-session-create-rate,omitempty"`
	MaxConnectionLifetime       *int `proxy:"max-connection-lifetime,omitempty"` // In seconds
	ConnectionLifetimeJitter    *int `proxy:"connection-lifetime-jitter,omitempty"` // In seconds
	IdleSessionTimeout          *int `proxy:"idle-session-timeout,omitempty"` // In seconds
	IdleSessionCheckInterval    *int `proxy:"idle-session-check-interval,omitempty"` // In seconds
}

func (t *AnyTLS) DialContext(ctx context.Context, metadata *C.Metadata) (_ C.Conn, err error) {
	c, err := t.client.CreateProxy(ctx, M.ParseSocksaddrHostPort(metadata.String(), metadata.DstPort))
	if err != nil {
		return nil, err
	}
	return NewConn(c, t), nil
}

func (t *AnyTLS) ListenPacketContext(ctx context.Context, metadata *C.Metadata) (_ C.PacketConn, err error) {
	if err = t.ResolveUDP(ctx, metadata); err != nil {
		return nil, err
	}

	// create tcp
	c, err := t.client.CreateProxy(ctx, uot.RequestDestination(2))
	if err != nil {
		return nil, err
	}

	// create uot on tcp
	destination := M.SocksaddrFromNet(metadata.UDPAddr())
	return newPacketConn(N.NewThreadSafePacketConn(uot.NewLazyConn(c, uot.Request{Destination: destination})), t), nil
}

// SupportUOT implements C.ProxyAdapter
func (t *AnyTLS) SupportUOT() bool {
	return true
}

// ProxyInfo implements C.ProxyAdapter
func (t *AnyTLS) ProxyInfo() C.ProxyInfo {
	info := t.Base.ProxyInfo()
	info.DialerProxy = t.option.DialerProxy
	return info
}

// Close implements C.ProxyAdapter
func (t *AnyTLS) Close() error {
	return t.client.Close()
}

func NewAnyTLS(option AnyTLSOption) (*AnyTLS, error) {
	addr := net.JoinHostPort(option.Server, strconv.Itoa(option.Port))
	outbound := &AnyTLS{
		Base: &Base{
			name:   option.Name,
			addr:   addr,
			tp:     C.AnyTLS,
			pdName: option.ProviderName,
			udp:    option.UDP,
			tfo:    option.TFO,
			mpTcp:  option.MPTCP,
			iface:  option.Interface,
			rmark:  option.RoutingMark,
			prefer: option.IPVersion,
		},
		option: &option,
	}
	outbound.dialer = option.NewDialer(outbound.DialOptions())
	singDialer := proxydialer.NewSingDialer(outbound.dialer)

	// Merge global config with per-proxy overrides
	tOption := anytls.ClientConfig{
		Password: option.Password,
		Server:   M.ParseSocksaddrHostPort(option.Server, uint16(option.Port)),
		Dialer:   singDialer,
	}

	// Apply global config if available
	globalCfg := getGlobalAnyTLSSessionConfig()
	if globalCfg != nil {
		tOption.IdleSessionCheckInterval = globalCfg.IdleSessionCheckInterval
		tOption.IdleSessionTimeout = globalCfg.IdleSessionTimeout
		tOption.MinIdleSession = globalCfg.MinIdleSession
		tOption.EnsureIdleSession = globalCfg.EnsureIdleSession
		tOption.EnsureIdleSessionCreateRate = globalCfg.EnsureIdleSessionCreateRate
		tOption.MinIdleSessionForAge = globalCfg.MinIdleSessionForAge
		tOption.MaxConnectionLifetime = globalCfg.MaxConnectionLifetime
		tOption.ConnectionLifetimeJitter = globalCfg.ConnectionLifetimeJitter
	}

	// Apply legacy per-proxy settings (backward compatibility)
	if option.IdleSessionCheckInterval > 0 {
		tOption.IdleSessionCheckInterval = time.Duration(option.IdleSessionCheckInterval) * time.Second
	}
	if option.IdleSessionTimeout > 0 {
		tOption.IdleSessionTimeout = time.Duration(option.IdleSessionTimeout) * time.Second
	}
	if option.MinIdleSession > 0 {
		tOption.MinIdleSession = option.MinIdleSession
	}

	// Apply per-proxy overrides (highest priority)
	if option.SessionOverride != nil {
		override := option.SessionOverride
		if override.EnsureIdleSession != nil {
			tOption.EnsureIdleSession = *override.EnsureIdleSession
		}
		if override.MinIdleSession != nil {
			tOption.MinIdleSession = *override.MinIdleSession
		}
		if override.MinIdleSessionForAge != nil {
			tOption.MinIdleSessionForAge = *override.MinIdleSessionForAge
		}
		if override.EnsureIdleSessionCreateRate != nil {
			tOption.EnsureIdleSessionCreateRate = *override.EnsureIdleSessionCreateRate
		}
		if override.MaxConnectionLifetime != nil {
			tOption.MaxConnectionLifetime = time.Duration(*override.MaxConnectionLifetime) * time.Second
		}
		if override.ConnectionLifetimeJitter != nil {
			tOption.ConnectionLifetimeJitter = time.Duration(*override.ConnectionLifetimeJitter) * time.Second
		}
		if override.IdleSessionTimeout != nil {
			tOption.IdleSessionTimeout = time.Duration(*override.IdleSessionTimeout) * time.Second
		}
		if override.IdleSessionCheckInterval != nil {
			tOption.IdleSessionCheckInterval = time.Duration(*override.IdleSessionCheckInterval) * time.Second
		}
	}

	echConfig, err := option.ECHOpts.Parse()
	if err != nil {
		return nil, err
	}
	tlsConfig := &vmess.TLSConfig{
		Host:              option.SNI,
		SkipCertVerify:    option.SkipCertVerify,
		NextProtos:        option.ALPN,
		FingerPrint:       option.Fingerprint,
		Certificate:       option.Certificate,
		PrivateKey:        option.PrivateKey,
		ClientFingerprint: option.ClientFingerprint,
		ECH:               echConfig,
	}
	if tlsConfig.Host == "" {
		tlsConfig.Host = option.Server
	}
	tOption.TLSConfig = tlsConfig

	client := anytls.NewClient(context.TODO(), tOption)
	outbound.client = client

	return outbound, nil
}
