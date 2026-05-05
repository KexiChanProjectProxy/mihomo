package outboundgroup

import (
	"testing"
	"time"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"
)

func TestParseLoadBalanceOption_LegacyMode(t *testing.T) {
	config := map[string]any{
		"type":      "load-balance",
		"name":      "test-lb",
		"proxies":   []any{"proxy-1", "proxy-2"},
		"strategy":  "consistent-hashing",
	}

	lbOpt, isPRDMode, err := parseLoadBalanceOption(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if isPRDMode {
		t.Error("expected legacy mode (isPRDMode=false)")
	}
	if lbOpt != nil {
		t.Error("expected nil LoadBalanceOption in legacy mode")
	}
}

func TestParseLoadBalanceOption_PRDBasic(t *testing.T) {
	config := map[string]any{
		"type":               "load-balance",
		"name":               "test-lb",
		"primary_outbounds":   []any{"proxy-1", "proxy-2"},
		"backup_outbounds":   []any{"backup-1"},
		"strategy":           "consistent_hash",
		"empty_pool_action":  "error",
		"interrupt_exist_connections": true,
		"prefer_domain":      true,
	}

	lbOpt, isPRDMode, err := parseLoadBalanceOption(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !isPRDMode {
		t.Error("expected PRD mode (isPRDMode=true)")
	}
	if lbOpt == nil {
		t.Fatal("expected non-nil LoadBalanceOption in PRD mode")
	}

	if len(lbOpt.PrimaryOutbounds) != 2 {
		t.Errorf("expected 2 primary_outbounds, got %d", len(lbOpt.PrimaryOutbounds))
	}
	if lbOpt.PrimaryOutbounds[0] != "proxy-1" {
		t.Errorf("expected primary_outbounds[0] to be 'proxy-1', got '%s'", lbOpt.PrimaryOutbounds[0])
	}
	if len(lbOpt.BackupOutbounds) != 1 {
		t.Errorf("expected 1 backup_outbound, got %d", len(lbOpt.BackupOutbounds))
	}
	if lbOpt.BackupOutbounds[0] != "backup-1" {
		t.Errorf("expected backup_outbounds[0] to be 'backup-1', got '%s'", lbOpt.BackupOutbounds[0])
	}
	if lbOpt.Strategy != "consistent_hash" {
		t.Errorf("expected strategy 'consistent_hash', got '%s'", lbOpt.Strategy)
	}
	if lbOpt.EmptyPoolAction != "error" {
		t.Errorf("expected empty_pool_action 'error', got '%s'", lbOpt.EmptyPoolAction)
	}
	if !lbOpt.InterruptExistConnections {
		t.Error("expected interrupt_exist_connections to be true")
	}
	if !lbOpt.PreferDomain {
		t.Error("expected prefer_domain to be true")
	}
}

func TestParseLoadBalanceOption_PRDWithNestedConfig(t *testing.T) {
	config := map[string]any{
		"type":             "load-balance",
		"name":             "test-lb",
		"primary_outbounds": []any{"proxy-1", "proxy-2"},
		"top_n": map[string]any{
			"primary": 3,
			"backup":  1,
		},
		"hash": map[string]any{
			"key_parts":     []any{"src_ip", "dst_ip"},
			"virtual_nodes": 100,
			"on_empty_key":  "random",
			"key_salt":      "test-salt",
		},
		"hysteresis": map[string]any{
			"primary_failures": 3,
			"backup_hold_time": "30s",
		},
		"strategy": "consistent_hash",
	}

	lbOpt, isPRDMode, err := parseLoadBalanceOption(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !isPRDMode {
		t.Error("expected PRD mode")
	}
	if lbOpt == nil {
		t.Fatal("expected non-nil LoadBalanceOption")
	}

	if lbOpt.TopN == nil {
		t.Fatal("expected non-nil TopN")
	}
	if lbOpt.TopN.Primary != 3 {
		t.Errorf("expected top_n.primary=3, got %d", lbOpt.TopN.Primary)
	}
	if lbOpt.TopN.Backup != 1 {
		t.Errorf("expected top_n.backup=1, got %d", lbOpt.TopN.Backup)
	}

	if lbOpt.Hash == nil {
		t.Fatal("expected non-nil Hash")
	}
	if len(lbOpt.Hash.KeyParts) != 2 {
		t.Errorf("expected 2 key_parts, got %d", len(lbOpt.Hash.KeyParts))
	}
	if lbOpt.Hash.VirtualNodes != 100 {
		t.Errorf("expected hash.virtual_nodes=100, got %d", lbOpt.Hash.VirtualNodes)
	}
	if lbOpt.Hash.OnEmptyKey != "random" {
		t.Errorf("expected hash.on_empty_key='random', got '%s'", lbOpt.Hash.OnEmptyKey)
	}
	if lbOpt.Hash.KeySalt != "test-salt" {
		t.Errorf("expected hash.key_salt='test-salt', got '%s'", lbOpt.Hash.KeySalt)
	}

	if lbOpt.Hysteresis == nil {
		t.Fatal("expected non-nil Hysteresis")
	}
	if lbOpt.Hysteresis.PrimaryFailures != 3 {
		t.Errorf("expected hysteresis.primary_failures=3, got %d", lbOpt.Hysteresis.PrimaryFailures)
	}
	if lbOpt.Hysteresis.BackupHoldTime != 30*time.Second {
		t.Errorf("expected hysteresis.backup_hold_time=30s, got %v", lbOpt.Hysteresis.BackupHoldTime)
	}
}

func TestParseLoadBalanceOption_PRDDefaultHash(t *testing.T) {
	config := map[string]any{
		"type":               "load-balance",
		"name":               "test-lb",
		"primary_outbounds":   []any{"proxy-1"},
	}

	lbOpt, isPRDMode, err := parseLoadBalanceOption(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !isPRDMode {
		t.Error("expected PRD mode")
	}

	if lbOpt.Hash != nil {
		t.Error("expected nil Hash when not specified")
	}
}

func TestParseLoadBalanceOption_MalformedPrimaryOutboundsEmpty(t *testing.T) {
	config := map[string]any{
		"type":               "load-balance",
		"name":               "test-lb",
		"primary_outbounds":   []any{},
	}

	_, _, err := parseLoadBalanceOption(config)
	if err == nil {
		t.Error("expected error for empty primary_outbounds")
	}
}

func TestParseLoadBalanceOption_MalformedPrimaryOutboundsNotArray(t *testing.T) {
	config := map[string]any{
		"type":               "load-balance",
		"name":               "test-lb",
		"primary_outbounds":   "not-an-array",
	}

	_, _, err := parseLoadBalanceOption(config)
	if err == nil {
		t.Error("expected error for non-array primary_outbounds")
	}
}

func TestParseLoadBalanceOption_MalformedTopNNotObject(t *testing.T) {
	config := map[string]any{
		"type":               "load-balance",
		"name":               "test-lb",
		"primary_outbounds":   []any{"proxy-1"},
		"top_n":              "not-an-object",
	}

	_, _, err := parseLoadBalanceOption(config)
	if err == nil {
		t.Error("expected error for non-object top_n")
	}
}

func TestParseLoadBalanceOption_MalformedHashNotObject(t *testing.T) {
	config := map[string]any{
		"type":               "load-balance",
		"name":               "test-lb",
		"primary_outbounds":   []any{"proxy-1"},
		"hash":               123,
	}

	_, _, err := parseLoadBalanceOption(config)
	if err == nil {
		t.Error("expected error for non-object hash")
	}
}

func TestParseLoadBalanceOption_MalformedHysteresisNotObject(t *testing.T) {
	config := map[string]any{
		"type":               "load-balance",
		"name":               "test-lb",
		"primary_outbounds":   []any{"proxy-1"},
		"hysteresis":         []any{"not", "an", "object"},
	}

	_, _, err := parseLoadBalanceOption(config)
	if err == nil {
		t.Error("expected error for non-object hysteresis")
	}
}

func TestParseLoadBalanceOption_MalformedEmptyPoolAction(t *testing.T) {
	config := map[string]any{
		"type":               "load-balance",
		"name":               "test-lb",
		"primary_outbounds":   []any{"proxy-1"},
		"empty_pool_action":   "invalid-action",
	}

	_, _, err := parseLoadBalanceOption(config)
	if err == nil {
		t.Error("expected error for invalid empty_pool_action")
	}
}

func TestParseLoadBalanceOption_MalformedTopNNegativePrimary(t *testing.T) {
	config := map[string]any{
		"type":               "load-balance",
		"name":               "test-lb",
		"primary_outbounds":   []any{"proxy-1"},
		"top_n": map[string]any{
			"primary": -1,
		},
	}

	_, _, err := parseLoadBalanceOption(config)
	if err == nil {
		t.Error("expected error for negative top_n.primary")
	}
}

func TestParseLoadBalanceOption_HysteresisNumericBackupHoldTime(t *testing.T) {
	config := map[string]any{
		"type":               "load-balance",
		"name":               "test-lb",
		"primary_outbounds":   []any{"proxy-1"},
		"hysteresis": map[string]any{
			"primary_failures": 3,
			"backup_hold_time":  30,
		},
	}

	lbOpt, _, err := parseLoadBalanceOption(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if lbOpt == nil {
		t.Fatal("expected non-nil LoadBalanceOption")
	}
	if lbOpt.Hysteresis == nil {
		t.Fatal("expected non-nil Hysteresis")
	}
	if lbOpt.Hysteresis.BackupHoldTime != 30*time.Second {
		t.Errorf("expected hysteresis.backup_hold_time=30s from numeric, got %v", lbOpt.Hysteresis.BackupHoldTime)
	}
}

func TestValidateLoadBalanceOption_Valid(t *testing.T) {
	opt := &LoadBalanceOption{
		PrimaryOutbounds: []string{"proxy-1", "proxy-2"},
		Strategy:         "consistent-hashing",
	}

	err := ValidateLoadBalanceOption(opt)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateLoadBalanceOption_ValidPRDStrategies(t *testing.T) {
	strategies := []string{"consistent-hashing", "round-robin", "sticky-sessions", "random", "consistent_hash"}

	for _, strategy := range strategies {
		opt := &LoadBalanceOption{
			PrimaryOutbounds: []string{"proxy-1"},
			Strategy:         strategy,
		}
		err := ValidateLoadBalanceOption(opt)
		if err != nil {
			t.Errorf("unexpected error for strategy '%s': %v", strategy, err)
		}
	}
}

func TestValidateLoadBalanceOption_EmptyPrimaryOutbounds(t *testing.T) {
	opt := &LoadBalanceOption{
		PrimaryOutbounds: []string{},
		Strategy:         "consistent-hashing",
	}

	err := ValidateLoadBalanceOption(opt)
	if err == nil {
		t.Error("expected error for empty primary_outbounds")
	}
}

func TestValidateLoadBalanceOption_InvalidStrategy(t *testing.T) {
	opt := &LoadBalanceOption{
		PrimaryOutbounds: []string{"proxy-1"},
		Strategy:         "invalid-strategy",
	}

	err := ValidateLoadBalanceOption(opt)
	if err == nil {
		t.Error("expected error for invalid strategy")
	}
}

func TestValidateLoadBalanceOption_InvalidEmptyPoolAction(t *testing.T) {
	opt := &LoadBalanceOption{
		PrimaryOutbounds: []string{"proxy-1"},
		Strategy:         "consistent-hashing",
		EmptyPoolAction:  "invalid",
	}

	err := ValidateLoadBalanceOption(opt)
	if err == nil {
		t.Error("expected error for invalid empty_pool_action")
	}
}

func TestValidateLoadBalanceOption_ValidEmptyPoolActions(t *testing.T) {
	actions := []string{"error", "select_next", "wait"}

	for _, action := range actions {
		opt := &LoadBalanceOption{
			PrimaryOutbounds: []string{"proxy-1"},
			Strategy:         "consistent-hashing",
			EmptyPoolAction:  action,
		}
		err := ValidateLoadBalanceOption(opt)
		if err != nil {
			t.Errorf("unexpected error for empty_pool_action '%s': %v", action, err)
		}
	}
}

func TestGetLoadBalanceStrategy(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"consistent_hash", "consistent-hashing"},
		{"consistent-hashing", "consistent-hashing"},
		{"round-robin", "round-robin"},
		{"sticky-sessions", "sticky-sessions"},
		{"random", "random"},
		{"", ""},
	}

	for _, test := range tests {
		result := GetLoadBalanceStrategy(test.input)
		if result != test.expected {
			t.Errorf("GetLoadBalanceStrategy(%q) = %q, want %q", test.input, result, test.expected)
		}
	}
}

func TestParseStrategy(t *testing.T) {
	tests := []struct {
		config   map[string]any
		expected string
	}{
		{map[string]any{"strategy": "consistent-hashing"}, "consistent-hashing"},
		{map[string]any{"strategy": "round-robin"}, "round-robin"},
		{map[string]any{"strategy": "random"}, "random"},
		{map[string]any{"strategy": "consistent_hash"}, "consistent_hash"},
		{map[string]any{}, "consistent-hashing"},
	}

	for _, test := range tests {
		result := parseStrategy(test.config)
		if result != test.expected {
			t.Errorf("parseStrategy(%v) = %q, want %q", test.config, result, test.expected)
		}
	}
}

func TestParseHysteresisOption_InvalidDuration(t *testing.T) {
	config := map[string]any{
		"primary_failures": 3,
		"backup_hold_time": "invalid-duration",
	}

	_, err := parseHysteresisOption(config)
	if err == nil {
		t.Error("expected error for invalid duration string")
	}
}

func TestParseTopNOption_InvalidPrimaryType(t *testing.T) {
	config := map[string]any{
		"primary": "not-an-int",
	}

	_, err := parseTopNOption(config)
	if err == nil {
		t.Error("expected error for non-int primary")
	}
}

func TestPRDModeDetection_WithPrimaryOutboundsOnly(t *testing.T) {
	config := map[string]any{
		"type":               "load-balance",
		"name":               "test-lb-prd",
		"primary_outbounds":   []any{"proxy-1", "proxy-2"},
	}

	lbOpt, isPRDMode, err := parseLoadBalanceOption(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !isPRDMode {
		t.Error("expected PRD mode when primary_outbounds is present")
	}
	if lbOpt == nil {
		t.Fatal("expected non-nil LoadBalanceOption")
	}
	if len(lbOpt.PrimaryOutbounds) != 2 {
		t.Errorf("expected 2 primary_outbounds, got %d", len(lbOpt.PrimaryOutbounds))
	}
}

func TestPRDModeDetection_NoProxiesOrUse(t *testing.T) {
	config := map[string]any{
		"type":      "load-balance",
		"name":      "test-lb",
		"proxies":   []any{},
		"use":       []any{},
	}

	_, isPRDMode, err := parseLoadBalanceOption(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if isPRDMode {
		t.Error("expected legacy mode when primary_outbounds is absent")
	}
}

func TestPRDModeDetection_ProxiesWithoutPrimaryOutbounds(t *testing.T) {
	config := map[string]any{
		"type":     "load-balance",
		"name":     "test-lb",
		"proxies":  []any{"proxy-1", "proxy-2"},
	}

	_, isPRDMode, err := parseLoadBalanceOption(config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if isPRDMode {
		t.Error("expected legacy mode when only proxies is present")
	}
}

func TestParseProxyGroup_LoadBalancePRD(t *testing.T) {
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
		"type":               "load-balance",
		"name":               "test-lb-prd",
		"primary_outbounds":   []any{"proxy-1", "proxy-2"},
		"strategy":           "consistent_hash",
		"empty_pool_action":  "error",
		"prefer_domain":      true,
	}

	group, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if group == nil {
		t.Fatal("expected non-nil group")
	}
	if group.Name() != "test-lb-prd" {
		t.Errorf("expected name 'test-lb-prd', got '%s'", group.Name())
	}
	if group.Type() != C.LoadBalance {
		t.Errorf("expected type LoadBalance, got %v", group.Type())
	}
}

func TestParseProxyGroup_LoadBalancePRDErrors(t *testing.T) {
	proxy1 := adapter.NewProxy(outbound.NewDirect())

	proxyMap := map[string]C.Proxy{
		"proxy-1": proxy1,
	}

	providersMap := map[string]P.ProxyProvider{}
	AllProxies := []string{}
	AllProviders := []string{}

	t.Run("malformed_primary_outbounds_empty", func(t *testing.T) {
		config := map[string]any{
			"type":               "load-balance",
			"name":               "test-lb",
			"primary_outbounds":   []any{},
		}
		_, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err == nil {
			t.Error("expected error for empty primary_outbounds")
		}
	})

	t.Run("malformed_top_n_not_object", func(t *testing.T) {
		config := map[string]any{
			"type":               "load-balance",
			"name":               "test-lb",
			"primary_outbounds":   []any{"proxy-1"},
			"top_n":             "not-an-object",
		}
		_, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err == nil {
			t.Error("expected error for non-object top_n")
		}
	})

	t.Run("malformed_hash_not_object", func(t *testing.T) {
		config := map[string]any{
			"type":               "load-balance",
			"name":               "test-lb",
			"primary_outbounds":   []any{"proxy-1"},
			"hash":              123,
		}
		_, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err == nil {
			t.Error("expected error for non-object hash")
		}
	})

	t.Run("malformed_invalid_empty_pool_action", func(t *testing.T) {
		config := map[string]any{
			"type":               "load-balance",
			"name":               "test-lb",
			"primary_outbounds":   []any{"proxy-1"},
			"empty_pool_action":  "invalid",
		}
		_, err := ParseProxyGroup(config, proxyMap, providersMap, AllProxies, AllProviders)
		if err == nil {
			t.Error("expected error for invalid empty_pool_action")
		}
	})
}