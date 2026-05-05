package outboundgroup

import (
	"net/netip"
	"testing"
	"time"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
)

func TestLoadBalanceRuntime(t *testing.T) {
	t.Run("PRD_mode_selection", func(t *testing.T) {
		proxy1 := adapter.NewProxy(outbound.NewDirect())
		proxy2 := adapter.NewProxy(outbound.NewDirect())

		proxyMap := map[string]C.Proxy{
			"proxy-1": proxy1,
			"proxy-2": proxy2,
		}

		providersMap := map[string]P.ProxyProvider{}
		AllProxies := []string{}
		AllProviders := []string{}

		config := map[string]any{
			"type":             "load-balance",
			"name":             "test-lb-prd",
			"primary_outbounds": []any{"proxy-1", "proxy-2"},
			"strategy":         "consistent_hash",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		if !lb.isPRD {
			t.Error("expected PRD mode to be enabled")
		}

		metadata := &C.Metadata{
			SrcIP:  netip.MustParseAddr("192.168.1.100"),
			DstIP:  netip.MustParseAddr("8.8.8.8"),
			SrcPort: 12345,
			DstPort: 443,
		}

		selected := lb.Unwrap(metadata, false)
		if selected == nil {
			t.Error("expected a proxy to be selected")
		}
	})

	t.Run("PRD_primary_only", func(t *testing.T) {
		proxy1 := adapter.NewProxy(outbound.NewDirect())

		proxyMap := map[string]C.Proxy{
			"proxy-1": proxy1,
		}

		providersMap := map[string]P.ProxyProvider{}
		AllProxies := []string{}
		AllProviders := []string{}

		config := map[string]any{
			"type":             "load-balance",
			"name":             "test-lb-primary",
			"primary_outbounds": []any{"proxy-1"},
			"strategy":         "round-robin",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		metadata := &C.Metadata{}

		for i := 0; i < 5; i++ {
			selected := lb.Unwrap(metadata, false)
			if selected == nil {
				t.Error("expected a proxy to be selected")
			}
			if selected.Name() != "DIRECT" {
				t.Errorf("expected DIRECT, got %s", selected.Name())
			}
		}
	})

	t.Run("PRD_with_backup", func(t *testing.T) {
		proxy1 := adapter.NewProxy(outbound.NewDirect())
		proxy2 := adapter.NewProxy(outbound.NewDirect())

		proxyMap := map[string]C.Proxy{
			"proxy-1": proxy1,
			"proxy-2": proxy2,
		}

		providersMap := map[string]P.ProxyProvider{}
		AllProxies := []string{}
		AllProviders := []string{}

		config := map[string]any{
			"type":              "load-balance",
			"name":              "test-lb-backup",
			"primary_outbounds":  []any{"proxy-1"},
			"backup_outbounds":   []any{"proxy-2"},
			"strategy":          "random",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		if len(lb.backupProxies) != 1 {
			t.Errorf("expected 1 backup proxy, got %d", len(lb.backupProxies))
		}

		metadata := &C.Metadata{}

		selected := lb.Unwrap(metadata, false)
		if selected == nil {
			t.Error("expected a proxy to be selected")
		}
	})
}

func TestLoadBalanceLegacyCompatibility(t *testing.T) {
	t.Run("legacy_consistent_hashing", func(t *testing.T) {
		proxy1 := adapter.NewProxy(outbound.NewDirect())
		proxy2 := adapter.NewProxy(outbound.NewDirect())

		proxyMap := map[string]C.Proxy{
			"proxy-1": proxy1,
			"proxy-2": proxy2,
		}

		providersMap := map[string]P.ProxyProvider{}
		AllProxies := []string{}
		AllProviders := []string{}

		config := map[string]any{
			"type":     "load-balance",
			"name":     "test-lb-legacy",
			"proxies":  []any{"proxy-1", "proxy-2"},
			"strategy": "consistent-hashing",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		if lb.isPRD {
			t.Error("legacy mode should not be PRD")
		}

		metadata := &C.Metadata{
			SrcIP: netip.MustParseAddr("192.168.1.100"),
			DstIP: netip.MustParseAddr("8.8.8.8"),
		}

		selected := lb.Unwrap(metadata, false)
		if selected == nil {
			t.Error("expected a proxy to be selected")
		}
	})

	t.Run("legacy_round_robin", func(t *testing.T) {
		proxy1 := adapter.NewProxy(outbound.NewDirect())
		proxy2 := adapter.NewProxy(outbound.NewDirect())

		proxyMap := map[string]C.Proxy{
			"proxy-1": proxy1,
			"proxy-2": proxy2,
		}

		providersMap := map[string]P.ProxyProvider{}
		AllProxies := []string{}
		AllProviders := []string{}

		config := map[string]any{
			"type":     "load-balance",
			"name":     "test-lb-rr",
			"proxies":  []any{"proxy-1", "proxy-2"},
			"strategy": "round-robin",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		metadata := &C.Metadata{}

		selected := lb.Unwrap(metadata, false)
		if selected == nil {
			t.Error("expected a proxy to be selected")
		}
	})

	t.Run("legacy_sticky_sessions", func(t *testing.T) {
		proxy1 := adapter.NewProxy(outbound.NewDirect())
		proxy2 := adapter.NewProxy(outbound.NewDirect())

		proxyMap := map[string]C.Proxy{
			"proxy-1": proxy1,
			"proxy-2": proxy2,
		}

		providersMap := map[string]P.ProxyProvider{}
		AllProxies := []string{}
		AllProviders := []string{}

		config := map[string]any{
			"type":     "load-balance",
			"name":     "test-lb-sticky",
			"proxies":  []any{"proxy-1", "proxy-2"},
			"strategy": "sticky-sessions",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		metadata := &C.Metadata{
			SrcIP: netip.MustParseAddr("192.168.1.100"),
			DstIP: netip.MustParseAddr("8.8.8.8"),
		}

		selected := lb.Unwrap(metadata, false)
		if selected == nil {
			t.Error("expected a proxy to be selected")
		}

		selected2 := lb.Unwrap(metadata, false)
		if selected2 == nil {
			t.Error("expected a proxy to be selected")
		}
	})
}

func TestLoadBalanceEmptyPool(t *testing.T) {
	t.Run("empty_primary_outbounds", func(t *testing.T) {
		proxyMap := map[string]C.Proxy{}

		providersMap := map[string]P.ProxyProvider{}
		AllProxies := []string{}
		AllProviders := []string{}

		config := map[string]any{
			"type":             "load-balance",
			"name":             "test-lb-empty",
			"primary_outbounds": []any{},
			"strategy":         "consistent_hash",
		}

		_, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err == nil {
			t.Error("expected error for empty primary_outbounds")
		}
	})

	t.Run("missing_proxy_in_primary", func(t *testing.T) {
		proxy1 := adapter.NewProxy(outbound.NewDirect())

		proxyMap := map[string]C.Proxy{
			"proxy-1": proxy1,
		}

		providersMap := map[string]P.ProxyProvider{}
		AllProxies := []string{}
		AllProviders := []string{}

		config := map[string]any{
			"type":             "load-balance",
			"name":             "test-lb-missing",
			"primary_outbounds": []any{"non-existent"},
			"strategy":         "consistent_hash",
		}

		_, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err == nil {
			t.Error("expected error for missing proxy in primary_outbounds")
		}
	})
}

func TestLoadBalancePRDOptionFields(t *testing.T) {
	t.Run("hash_tolerance_parsed", func(t *testing.T) {
		proxy1 := adapter.NewProxy(outbound.NewDirect())

		proxyMap := map[string]C.Proxy{
			"proxy-1": proxy1,
		}

		providersMap := map[string]P.ProxyProvider{}
		AllProxies := []string{}
		AllProviders := []string{}

		config := map[string]any{
			"type":             "load-balance",
			"name":             "test-lb-tolerance",
			"primary_outbounds": []any{"proxy-1"},
			"strategy":         "consistent_hash",
			"hash": map[string]any{
				"tolerance": 50,
			},
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		if lb.lbOpt == nil || lb.lbOpt.Hash == nil {
			t.Fatal("expected hash options to be set")
		}

		if lb.lbOpt.Hash.Tolerance != 50 {
			t.Errorf("expected tolerance 50, got %d", lb.lbOpt.Hash.Tolerance)
		}
	})

	t.Run("top_n_parsed", func(t *testing.T) {
		proxy1 := adapter.NewProxy(outbound.NewDirect())

		proxyMap := map[string]C.Proxy{
			"proxy-1": proxy1,
		}

		providersMap := map[string]P.ProxyProvider{}
		AllProxies := []string{}
		AllProviders := []string{}

		config := map[string]any{
			"type":             "load-balance",
			"name":             "test-lb-topn",
			"primary_outbounds": []any{"proxy-1"},
			"strategy":         "consistent_hash",
			"top_n": map[string]any{
				"primary": 2,
				"backup":  1,
			},
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		if lb.lbOpt == nil || lb.lbOpt.TopN == nil {
			t.Fatal("expected top_n options to be set")
		}

		if lb.lbOpt.TopN.Primary != 2 {
			t.Errorf("expected primary 2, got %d", lb.lbOpt.TopN.Primary)
		}

		if lb.lbOpt.TopN.Backup != 1 {
			t.Errorf("expected backup 1, got %d", lb.lbOpt.TopN.Backup)
		}
	})

	t.Run("hysteresis_parsed", func(t *testing.T) {
		proxy1 := adapter.NewProxy(outbound.NewDirect())

		proxyMap := map[string]C.Proxy{
			"proxy-1": proxy1,
		}

		providersMap := map[string]P.ProxyProvider{}
		AllProxies := []string{}
		AllProviders := []string{}

		config := map[string]any{
			"type":             "load-balance",
			"name":             "test-lb-hyst",
			"primary_outbounds": []any{"proxy-1"},
			"strategy":         "consistent_hash",
			"hysteresis": map[string]any{
				"primary_failures": 5,
				"backup_hold_time": "10s",
			},
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		if lb.lbOpt == nil || lb.lbOpt.Hysteresis == nil {
			t.Fatal("expected hysteresis options to be set")
		}

		if lb.lbOpt.Hysteresis.PrimaryFailures != 5 {
			t.Errorf("expected primary_failures 5, got %d", lb.lbOpt.Hysteresis.PrimaryFailures)
		}

		if lb.lbOpt.Hysteresis.BackupHoldTime != time.Second*10 {
			t.Errorf("expected backup_hold_time 10s, got %v", lb.lbOpt.Hysteresis.BackupHoldTime)
		}
	})
}

func TestLoadBalancePRDState(t *testing.T) {
	t.Run("hyst_state_initialized", func(t *testing.T) {
		proxy1 := adapter.NewProxy(outbound.NewDirect())

		proxyMap := map[string]C.Proxy{
			"proxy-1": proxy1,
		}

		providersMap := map[string]P.ProxyProvider{}
		AllProxies := []string{}
		AllProviders := []string{}

		config := map[string]any{
			"type":             "load-balance",
			"name":             "test-lb-state",
			"primary_outbounds": []any{"proxy-1"},
			"strategy":         "round-robin",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		if lb.hystState == nil {
			t.Error("expected hysteresis state to be initialized")
		}

		if !IsPrimaryActive(lb.hystState) {
			t.Error("expected primary to be active initially")
		}

		if lb.candidateSingle == nil {
			t.Error("expected candidate single to be initialized")
		}
	})

	t.Run("candidate_single_works", func(t *testing.T) {
		proxy1 := adapter.NewProxy(outbound.NewDirect())

		proxyMap := map[string]C.Proxy{
			"proxy-1": proxy1,
		}

		providersMap := map[string]P.ProxyProvider{}
		AllProxies := []string{}
		AllProviders := []string{}

		config := map[string]any{
			"type":             "load-balance",
			"name":             "test-lb-candidate",
			"primary_outbounds": []any{"proxy-1"},
			"strategy":         "round-robin",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		metadata := &C.Metadata{}

		selected1 := lb.Unwrap(metadata, true)
		if selected1 == nil {
			t.Error("expected a proxy to be selected")
		}

		selected2 := lb.Unwrap(metadata, true)
		if selected2 == nil {
			t.Error("expected a proxy to be selected")
		}

		if selected1.Name() != selected2.Name() {
			t.Error("expected same proxy to be selected within cache window")
		}
	})
}

func TestLoadBalanceMarshalJSON(t *testing.T) {
	proxy1 := adapter.NewProxy(outbound.NewDirect())
	proxy2 := adapter.NewProxy(outbound.NewDirect())

	proxyMap := map[string]C.Proxy{
		"proxy-1": proxy1,
		"proxy-2": proxy2,
	}

	providersMap := map[string]P.ProxyProvider{}
	AllProxies := []string{}
	AllProviders := []string{}

	t.Run("legacy_mode_json", func(t *testing.T) {
		config := map[string]any{
			"type":     "load-balance",
			"name":     "test-lb-json",
			"proxies":  []any{"proxy-1", "proxy-2"},
			"strategy": "consistent-hashing",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, err := group.MarshalJSON()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(data) == 0 {
			t.Error("expected non-empty JSON")
		}
	})

	t.Run("prd_mode_json", func(t *testing.T) {
		config := map[string]any{
			"type":             "load-balance",
			"name":             "test-lb-json-prd",
			"primary_outbounds": []any{"proxy-1", "proxy-2"},
			"strategy":         "consistent_hash",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		data, err := group.MarshalJSON()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(data) == 0 {
			t.Error("expected non-empty JSON")
		}
	})
}

func TestLoadBalanceProxies(t *testing.T) {
	proxy1 := adapter.NewProxy(outbound.NewDirect())
	proxy2 := adapter.NewProxy(outbound.NewDirect())

	proxyMap := map[string]C.Proxy{
		"proxy-1": proxy1,
		"proxy-2": proxy2,
	}

	providersMap := map[string]P.ProxyProvider{}
	AllProxies := []string{}
	AllProviders := []string{}

	t.Run("legacy_proxies", func(t *testing.T) {
		config := map[string]any{
			"type":     "load-balance",
			"name":     "test-lb-proxies",
			"proxies":  []any{"proxy-1", "proxy-2"},
			"strategy": "round-robin",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		proxies := lb.Proxies()
		if len(proxies) != 2 {
			t.Errorf("expected 2 proxies, got %d", len(proxies))
		}
	})

	t.Run("prd_proxies", func(t *testing.T) {
		config := map[string]any{
			"type":             "load-balance",
			"name":             "test-lb-proxies-prd",
			"primary_outbounds": []any{"proxy-1", "proxy-2"},
			"strategy":         "round-robin",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		proxies := lb.Proxies()
		if len(proxies) != 2 {
			t.Errorf("expected 2 proxies, got %d", len(proxies))
		}
	})
}

func TestLoadBalanceProviders(t *testing.T) {
	proxy1 := adapter.NewProxy(outbound.NewDirect())

	proxyMap := map[string]C.Proxy{
		"proxy-1": proxy1,
	}

	providersMap := map[string]P.ProxyProvider{}
	AllProxies := []string{}
	AllProviders := []string{}

	config := map[string]any{
		"type":             "load-balance",
		"name":             "test-lb-providers",
		"primary_outbounds": []any{"proxy-1"},
		"strategy":         "round-robin",
	}

	group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lb, ok := group.(*LoadBalance)
	if !ok {
		t.Fatal("expected LoadBalance type")
	}

	providers := lb.Providers()
	if len(providers) == 0 {
		t.Error("expected at least one provider")
	}
}

func TestLoadBalanceSupportUDP(t *testing.T) {
	proxy1 := adapter.NewProxy(outbound.NewDirect())

	proxyMap := map[string]C.Proxy{
		"proxy-1": proxy1,
	}

	providersMap := map[string]P.ProxyProvider{}
	AllProxies := []string{}
	AllProviders := []string{}

	t.Run("udp_enabled_by_default", func(t *testing.T) {
		config := map[string]any{
			"type":             "load-balance",
			"name":             "test-lb-udp",
			"primary_outbounds": []any{"proxy-1"},
			"strategy":         "round-robin",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !group.SupportUDP() {
			t.Error("expected UDP to be supported")
		}
	})

	t.Run("udp_disabled", func(t *testing.T) {
		config := map[string]any{
			"type":              "load-balance",
			"name":              "test-lb-udp-disabled",
			"primary_outbounds":  []any{"proxy-1"},
			"strategy":          "round-robin",
			"disable-udp":       true,
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if group.SupportUDP() {
			t.Error("expected UDP to be disabled")
		}
	})
}

func TestLoadBalanceEmptyPoolAction(t *testing.T) {
	t.Run("empty_pool_action_error_returns_nil", func(t *testing.T) {
		proxy1 := adapter.NewProxy(outbound.NewDirect())

		proxyMap := map[string]C.Proxy{
			"proxy-1": proxy1,
		}

		providersMap := map[string]P.ProxyProvider{}
		AllProxies := []string{}
		AllProviders := []string{}

		config := map[string]any{
			"type":              "load-balance",
			"name":              "test-lb-empty-error",
			"primary_outbounds":  []any{"proxy-1"},
			"strategy":          "consistent_hash",
			"empty_pool_action":  "error",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		if lb.lbOpt.EmptyPoolAction != "error" {
			t.Errorf("expected empty_pool_action 'error', got '%s'", lb.lbOpt.EmptyPoolAction)
		}
	})

	t.Run("empty_pool_action_select_next_fallback", func(t *testing.T) {
		proxy1 := adapter.NewProxy(outbound.NewDirect())
		proxy2 := adapter.NewProxy(outbound.NewDirect())

		proxyMap := map[string]C.Proxy{
			"proxy-1": proxy1,
			"proxy-2": proxy2,
		}

		providersMap := map[string]P.ProxyProvider{}
		AllProxies := []string{}
		AllProviders := []string{}

		config := map[string]any{
			"type":              "load-balance",
			"name":              "test-lb-empty-select-next",
			"primary_outbounds":  []any{"proxy-1"},
			"backup_outbounds":   []any{"proxy-2"},
			"strategy":          "consistent_hash",
			"empty_pool_action":  "select_next",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		if lb.lbOpt.EmptyPoolAction != "select_next" {
			t.Errorf("expected empty_pool_action 'select_next', got '%s'", lb.lbOpt.EmptyPoolAction)
		}
	})
}

func TestLoadBalancePreferDomain(t *testing.T) {
	t.Run("prefer_domain_parsed", func(t *testing.T) {
		proxy1 := adapter.NewProxy(outbound.NewDirect())

		proxyMap := map[string]C.Proxy{
			"proxy-1": proxy1,
		}

		providersMap := map[string]P.ProxyProvider{}
		AllProxies := []string{}
		AllProviders := []string{}

		config := map[string]any{
			"type":             "load-balance",
			"name":             "test-lb-prefer-domain",
			"primary_outbounds": []any{"proxy-1"},
			"strategy":         "consistent_hash",
			"prefer_domain":    true,
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		if !lb.lbOpt.PreferDomain {
			t.Error("expected prefer_domain to be true")
		}
	})

	t.Run("prefer_domain_affects_selection", func(t *testing.T) {
		proxy1 := adapter.NewProxy(outbound.NewDirect())
		proxy2 := adapter.NewProxy(outbound.NewDirect())

		proxyMap := map[string]C.Proxy{
			"proxy-1": proxy1,
			"proxy-2": proxy2,
		}

		providersMap := map[string]P.ProxyProvider{}
		AllProxies := []string{}
		AllProviders := []string{}

		config := map[string]any{
			"type":              "load-balance",
			"name":              "test-lb-domain-affinity",
			"primary_outbounds":  []any{"proxy-1", "proxy-2"},
			"strategy":          "consistent_hash",
			"prefer_domain":      true,
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		metadataWithDomain := &C.Metadata{
			Host: "example.com",
			DstIP: netip.MustParseAddr("8.8.8.8"),
		}

		selected := lb.Unwrap(metadataWithDomain, false)
		if selected == nil {
			t.Error("expected a proxy to be selected with domain metadata")
		}

		metadataWithoutDomain := &C.Metadata{
			DstIP: netip.MustParseAddr("8.8.8.8"),
		}

		selected2 := lb.Unwrap(metadataWithoutDomain, false)
		if selected2 == nil {
			t.Error("expected a proxy to be selected without domain metadata")
		}
	})
}

func TestLoadBalanceInterruptConnections(t *testing.T) {
	t.Run("interrupt_exist_connections_parsed_and_stored", func(t *testing.T) {
		proxy1 := adapter.NewProxy(outbound.NewDirect())

		proxyMap := map[string]C.Proxy{
			"proxy-1": proxy1,
		}

		providersMap := map[string]P.ProxyProvider{}
		AllProxies := []string{}
		AllProviders := []string{}

		config := map[string]any{
			"type":                       "load-balance",
			"name":                       "test-lb-iec",
			"primary_outbounds":           []any{"proxy-1"},
			"strategy":                   "consistent_hash",
			"interrupt_exist_connections": true,
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		if !lb.lbOpt.InterruptExistConnections {
			t.Error("expected interrupt_exist_connections to be parsed and stored")
		}
	})

	t.Run("interrupt_exist_connections_false_by_default", func(t *testing.T) {
		proxy1 := adapter.NewProxy(outbound.NewDirect())

		proxyMap := map[string]C.Proxy{
			"proxy-1": proxy1,
		}

		providersMap := map[string]P.ProxyProvider{}
		AllProxies := []string{}
		AllProviders := []string{}

		config := map[string]any{
			"type":             "load-balance",
			"name":             "test-lb-iec-false",
			"primary_outbounds": []any{"proxy-1"},
			"strategy":         "consistent_hash",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		if lb.lbOpt.InterruptExistConnections {
			t.Error("expected interrupt_exist_connections to be false by default")
		}
	})

	t.Run("no_runtime_effect_at_proxy_group_layer", func(t *testing.T) {
		proxy1 := adapter.NewProxy(outbound.NewDirect())

		proxyMap := map[string]C.Proxy{
			"proxy-1": proxy1,
		}

		providersMap := map[string]P.ProxyProvider{}
		AllProxies := []string{}
		AllProviders := []string{}

		config := map[string]any{
			"type":                       "load-balance",
			"name":                       "test-lb-iec-no-effect",
			"primary_outbounds":           []any{"proxy-1"},
			"strategy":                   "consistent_hash",
			"interrupt_exist_connections": true,
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		if !lb.lbOpt.InterruptExistConnections {
			t.Error("flag should be stored as true")
		}

		if !lb.isPRD {
			t.Fatal("expected PRD mode for this test")
		}
	})
}

func TestLoadBalanceHealthCheckPRD(t *testing.T) {
	t.Run("healthCheckPRD_wired_for_prd_mode", func(t *testing.T) {
		proxy1 := adapter.NewProxy(outbound.NewDirect())

		proxyMap := map[string]C.Proxy{
			"proxy-1": proxy1,
		}

		providersMap := map[string]P.ProxyProvider{}
		AllProxies := []string{}
		AllProviders := []string{}

		config := map[string]any{
			"type":             "load-balance",
			"name":             "test-lb-hc-prd",
			"primary_outbounds": []any{"proxy-1"},
			"strategy":         "round-robin",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		if lb.hystState == nil {
			t.Error("expected hystState to be initialized for PRD mode")
		}

		if lb.candidateSingle == nil {
			t.Error("expected candidateSingle to be initialized for PRD mode")
		}

		if !lb.isPRD {
			t.Error("expected isPRD to be true")
		}
	})
}

func TestLoadBalancePRDSelectionCache(t *testing.T) {
	t.Run("selection_cached_within_time_window", func(t *testing.T) {
		proxy1 := adapter.NewProxy(outbound.NewDirect())

		proxyMap := map[string]C.Proxy{
			"proxy-1": proxy1,
		}

		providersMap := map[string]P.ProxyProvider{}
		AllProxies := []string{}
		AllProviders := []string{}

		config := map[string]any{
			"type":             "load-balance",
			"name":             "test-lb-cache",
			"primary_outbounds": []any{"proxy-1"},
			"strategy":         "round-robin",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		metadata := &C.Metadata{}

		selected1 := lb.prdUnwrap(metadata, true)
		selected2 := lb.prdUnwrap(metadata, true)

		if selected1.Name() != selected2.Name() {
			t.Error("expected same proxy within cache window")
		}
	})

	t.Run("selection_refreshed_after_reset", func(t *testing.T) {
		proxy1 := adapter.NewProxy(outbound.NewDirect())

		proxyMap := map[string]C.Proxy{
			"proxy-1": proxy1,
		}

		providersMap := map[string]P.ProxyProvider{}
		AllProxies := []string{}
		AllProviders := []string{}

		config := map[string]any{
			"type":             "load-balance",
			"name":             "test-lb-reset",
			"primary_outbounds": []any{"proxy-1"},
			"strategy":         "round-robin",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		metadata := &C.Metadata{}

		lb.prdUnwrap(metadata, true)
		lb.candidateSingle.Reset()
		lb.prdUnwrap(metadata, true)

		if lb.candidateSingle == nil {
			t.Error("expected candidateSingle to still exist after reset")
		}
	})
}

func TestLoadBalancePRDRuntimeStrategies(t *testing.T) {
	proxy1 := adapter.NewProxy(outbound.NewDirect())
	proxy2 := adapter.NewProxy(outbound.NewDirect())

	proxyMap := map[string]C.Proxy{
		"proxy-1": proxy1,
		"proxy-2": proxy2,
	}

	providersMap := map[string]P.ProxyProvider{}
	AllProxies := []string{}
	AllProviders := []string{}

	t.Run("PRD_random_strategy_selects_proxies", func(t *testing.T) {
		config := map[string]any{
			"type":              "load-balance",
			"name":              "test-lb-random",
			"primary_outbounds":  []any{"proxy-1", "proxy-2"},
			"strategy":          "random",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		metadata := &C.Metadata{}

		// Random should select something
		selected := lb.Unwrap(metadata, false)
		if selected == nil {
			t.Error("expected a proxy to be selected with random strategy")
		}
	})

	t.Run("PRD_consistent_hash_with_custom_key_parts", func(t *testing.T) {
		config := map[string]any{
			"type":              "load-balance",
			"name":              "test-lb-ch-key",
			"primary_outbounds":  []any{"proxy-1", "proxy-2"},
			"strategy":          "consistent_hash",
			"hash": map[string]any{
				"key_parts":   []any{"src_ip", "dst_ip"},
				"key_salt":    "test-",
				"virtual_nodes": 50,
			},
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		if lb.lbOpt == nil || lb.lbOpt.Hash == nil {
			t.Fatal("expected hash options to be set")
		}

		if len(lb.lbOpt.Hash.KeyParts) != 2 {
			t.Errorf("expected 2 key parts, got %d", len(lb.lbOpt.Hash.KeyParts))
		}

		if lb.lbOpt.Hash.KeySalt != "test-" {
			t.Errorf("expected key salt 'test-', got '%s'", lb.lbOpt.Hash.KeySalt)
		}

		if lb.lbOpt.Hash.VirtualNodes != 50 {
			t.Errorf("expected 50 virtual nodes, got %d", lb.lbOpt.Hash.VirtualNodes)
		}

		metadata := &C.Metadata{
			SrcIP: netip.MustParseAddr("192.168.1.100"),
			DstIP: netip.MustParseAddr("8.8.8.8"),
		}

		selected := lb.Unwrap(metadata, false)
		if selected == nil {
			t.Error("expected a proxy to be selected")
		}
	})

	t.Run("PRD_consistent_hash_deterministic", func(t *testing.T) {
		config := map[string]any{
			"type":              "load-balance",
			"name":              "test-lb-ch-det",
			"primary_outbounds":  []any{"proxy-1", "proxy-2"},
			"strategy":          "consistent_hash",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		metadata := &C.Metadata{
			SrcIP: netip.MustParseAddr("192.168.1.100"),
			DstIP: netip.MustParseAddr("8.8.8.8"),
		}

		// Same metadata should always return same proxy
		var firstName string
		for i := 0; i < 10; i++ {
			selected := lb.Unwrap(metadata, false)
			if selected == nil {
				t.Fatal("expected a proxy to be selected")
			}
			if i == 0 {
				firstName = selected.Name()
			} else if selected.Name() != firstName {
				t.Errorf("consistent hash should be deterministic, got %s then %s", firstName, selected.Name())
			}
		}
	})

	t.Run("PRD_round_robin_iterates", func(t *testing.T) {
		config := map[string]any{
			"type":              "load-balance",
			"name":              "test-lb-rr-iter",
			"primary_outbounds":  []any{"proxy-1", "proxy-2"},
			"strategy":          "round-robin",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		metadata := &C.Metadata{}

		// Round robin should iterate
		names := make(map[string]bool)
		for i := 0; i < 10; i++ {
			selected := lb.Unwrap(metadata, true) // touch=true to advance
			if selected != nil {
				names[selected.Name()] = true
			}
		}

		// Should see at least one proxy
		if len(names) == 0 {
			t.Error("expected at least one proxy to be selected")
		}
	})

	t.Run("PRD_sticky_sessions_same_connection", func(t *testing.T) {
		config := map[string]any{
			"type":              "load-balance",
			"name":              "test-lb-sticky-prd",
			"primary_outbounds":  []any{"proxy-1", "proxy-2"},
			"strategy":          "sticky-sessions",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		metadata := &C.Metadata{
			SrcIP: netip.MustParseAddr("192.168.1.100"),
			DstIP: netip.MustParseAddr("8.8.8.8"),
		}

		// Same metadata should return same proxy
		selected1 := lb.Unwrap(metadata, false)
		selected2 := lb.Unwrap(metadata, false)

		if selected1 == nil || selected2 == nil {
			t.Fatal("expected proxies to be selected")
		}

		if selected1.Name() != selected2.Name() {
			t.Errorf("sticky sessions should return same proxy, got %s and %s", selected1.Name(), selected2.Name())
		}
	})
}

func TestLoadBalanceTopNWithBackup(t *testing.T) {
	proxy1 := adapter.NewProxy(outbound.NewDirect())
	proxy2 := adapter.NewProxy(outbound.NewDirect())
	proxy3 := adapter.NewProxy(outbound.NewDirect())

	proxyMap := map[string]C.Proxy{
		"proxy-1": proxy1,
		"proxy-2": proxy2,
		"proxy-3": proxy3,
	}

	providersMap := map[string]P.ProxyProvider{}
	AllProxies := []string{}
	AllProviders := []string{}

	t.Run("top_n_backup_selects_from_backup", func(t *testing.T) {
		config := map[string]any{
			"type":              "load-balance",
			"name":              "test-lb-topn-backup",
			"primary_outbounds":  []any{"proxy-1", "proxy-2"},
			"backup_outbounds":   []any{"proxy-3"},
			"strategy":          "round-robin",
			"top_n": map[string]any{
				"primary": 1,
				"backup":  1,
			},
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		if lb.lbOpt == nil || lb.lbOpt.TopN == nil {
			t.Fatal("expected top_n to be set")
		}

		if lb.lbOpt.TopN.Backup != 1 {
			t.Errorf("expected top_n.backup=1, got %d", lb.lbOpt.TopN.Backup)
		}

		// Verify backup proxy is set
		if len(lb.backupProxies) != 1 {
			t.Errorf("expected 1 backup proxy, got %d", len(lb.backupProxies))
		}
	})
}

func TestLoadBalanceHysteresisRuntime(t *testing.T) {
	proxy1 := adapter.NewProxy(outbound.NewDirect())
	proxy2 := adapter.NewProxy(outbound.NewDirect())

	proxyMap := map[string]C.Proxy{
		"proxy-1": proxy1,
		"proxy-2": proxy2,
	}

	providersMap := map[string]P.ProxyProvider{}
	AllProxies := []string{}
	AllProviders := []string{}

	t.Run("hysteresis_initial_primary_active", func(t *testing.T) {
		config := map[string]any{
			"type":              "load-balance",
			"name":              "test-lb-hyst-init",
			"primary_outbounds":  []any{"proxy-1"},
			"backup_outbounds":   []any{"proxy-2"},
			"strategy":          "round-robin",
			"hysteresis": map[string]any{
				"primary_failures": 3,
				"backup_hold_time": "10s",
			},
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		if lb.hystState == nil {
			t.Fatal("expected hystState to be initialized")
		}

		if !IsPrimaryActive(lb.hystState) {
			t.Error("expected primary to be active initially")
		}
	})

	t.Run("hysteresis_backup_hold_time_parsed", func(t *testing.T) {
		config := map[string]any{
			"type":              "load-balance",
			"name":              "test-lb-hyst-time",
			"primary_outbounds":  []any{"proxy-1"},
			"backup_outbounds":   []any{"proxy-2"},
			"strategy":          "round-robin",
			"hysteresis": map[string]any{
				"primary_failures": 5,
				"backup_hold_time": "30s",
			},
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		if lb.lbOpt == nil || lb.lbOpt.Hysteresis == nil {
			t.Fatal("expected hysteresis to be set")
		}

		if lb.lbOpt.Hysteresis.PrimaryFailures != 5 {
			t.Errorf("expected primary_failures=5, got %d", lb.lbOpt.Hysteresis.PrimaryFailures)
		}

		if lb.lbOpt.Hysteresis.BackupHoldTime != 30*time.Second {
			t.Errorf("expected backup_hold_time=30s, got %v", lb.lbOpt.Hysteresis.BackupHoldTime)
		}
	})
}

func TestLoadBalanceEmptyPoolActionWait(t *testing.T) {
	proxy1 := adapter.NewProxy(outbound.NewDirect())

	proxyMap := map[string]C.Proxy{
		"proxy-1": proxy1,
	}

	providersMap := map[string]P.ProxyProvider{}
	AllProxies := []string{}
	AllProviders := []string{}

	t.Run("wait_action_parsed", func(t *testing.T) {
		config := map[string]any{
			"type":              "load-balance",
			"name":              "test-lb-wait",
			"primary_outbounds":  []any{"proxy-1"},
			"strategy":          "consistent_hash",
			"empty_pool_action":  "wait",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		if lb.lbOpt.EmptyPoolAction != "wait" {
			t.Errorf("expected empty_pool_action 'wait', got '%s'", lb.lbOpt.EmptyPoolAction)
		}
	})
}

func TestLoadBalanceBackupFallback(t *testing.T) {
	proxy1 := adapter.NewProxy(outbound.NewDirect())
	proxy2 := adapter.NewProxy(outbound.NewDirect())

	proxyMap := map[string]C.Proxy{
		"proxy-1": proxy1,
		"proxy-2": proxy2,
	}

	providersMap := map[string]P.ProxyProvider{}
	AllProxies := []string{}
	AllProviders := []string{}

	t.Run("backup_proxy_accessible", func(t *testing.T) {
		config := map[string]any{
			"type":              "load-balance",
			"name":              "test-lb-backup-access",
			"primary_outbounds":  []any{"proxy-1"},
			"backup_outbounds":   []any{"proxy-2"},
			"strategy":          "round-robin",
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		if len(lb.backupProxies) != 1 {
			t.Errorf("expected 1 backup proxy, got %d", len(lb.backupProxies))
		}

		if lb.backupProxies[0] == nil {
			t.Error("expected backup proxy to not be nil")
		}
	})
}

func TestLoadBalanceHashVirtualNodes(t *testing.T) {
	proxy1 := adapter.NewProxy(outbound.NewDirect())
	proxy2 := adapter.NewProxy(outbound.NewDirect())

	proxyMap := map[string]C.Proxy{
		"proxy-1": proxy1,
		"proxy-2": proxy2,
	}

	providersMap := map[string]P.ProxyProvider{}
	AllProxies := []string{}
	AllProviders := []string{}

	t.Run("hash_virtual_nodes_zero", func(t *testing.T) {
		config := map[string]any{
			"type":              "load-balance",
			"name":              "test-lb-vn-zero",
			"primary_outbounds":  []any{"proxy-1", "proxy-2"},
			"strategy":          "consistent_hash",
			"hash": map[string]any{
				"virtual_nodes": 0,
			},
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		if lb.lbOpt == nil || lb.lbOpt.Hash == nil {
			t.Fatal("expected hash options to be set")
		}

		if lb.lbOpt.Hash.VirtualNodes != 0 {
			t.Errorf("expected 0 virtual nodes, got %d", lb.lbOpt.Hash.VirtualNodes)
		}

		metadata := &C.Metadata{
			SrcIP: netip.MustParseAddr("192.168.1.100"),
			DstIP: netip.MustParseAddr("8.8.8.8"),
		}

		selected := lb.Unwrap(metadata, false)
		if selected == nil {
			t.Error("expected a proxy to be selected")
		}
	})

	t.Run("hash_on_empty_key_fail", func(t *testing.T) {
		config := map[string]any{
			"type":              "load-balance",
			"name":              "test-lb-on-empty-fail",
			"primary_outbounds":  []any{"proxy-1", "proxy-2"},
			"strategy":          "consistent_hash",
			"hash": map[string]any{
				"key_parts":   []any{"src_ip", "dst_ip"},
				"on_empty_key": "fail",
			},
		}

		group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		lb, ok := group.(*LoadBalance)
		if !ok {
			t.Fatal("expected LoadBalance type")
		}

		if lb.lbOpt == nil || lb.lbOpt.Hash == nil {
			t.Fatal("expected hash options to be set")
		}

		if lb.lbOpt.Hash.OnEmptyKey != "fail" {
			t.Errorf("expected on_empty_key 'fail', got '%s'", lb.lbOpt.Hash.OnEmptyKey)
		}
	})
}
