package outboundgroup

import (
	"net/netip"
	"testing"
	"time"

	C "github.com/metacubex/mihomo/constant"
)

func TestBuildHashRing(t *testing.T) {
	members := []string{"node-a", "node-b", "node-c"}

	ring := BuildHashRing(members, 100)

	if HashRingMemberCount(ring) != 3 {
		t.Errorf("expected 3 members, got %d", HashRingMemberCount(ring))
	}

	if HashRingPointCount(ring) != 300 {
		t.Errorf("expected 300 points (3 members * 100 virtual nodes), got %d", HashRingPointCount(ring))
	}
}

func TestBuildHashRing_ZeroVirtualNodes(t *testing.T) {
	members := []string{"node-a", "node-b"}

	ring := BuildHashRing(members, 0)

	if HashRingMemberCount(ring) != 2 {
		t.Errorf("expected 2 members, got %d", HashRingMemberCount(ring))
	}

	if HashRingPointCount(ring) != 0 {
		t.Errorf("expected 0 points with 0 virtual nodes, got %d", HashRingPointCount(ring))
	}
}

func TestBuildHashRing_EmptyMembers(t *testing.T) {
	ring := BuildHashRing([]string{}, 100)

	if HashRingMemberCount(ring) != 0 {
		t.Errorf("expected 0 members, got %d", HashRingMemberCount(ring))
	}

	if HashRingPointCount(ring) != 0 {
		t.Errorf("expected 0 points, got %d", HashRingPointCount(ring))
	}

	// Lookup should return empty string
	result := LookupHashRing(ring, 12345)
	if result != "" {
		t.Errorf("expected empty string for empty ring, got %s", result)
	}
}

func TestLookupHashRing(t *testing.T) {
	members := []string{"node-a", "node-b", "node-c"}
	ring := BuildHashRing(members, 100)

	// Test deterministic lookup - same key should always return same node
	key1 := ComputeHashFromKey("test-key-1")
	key2 := ComputeHashFromKey("test-key-2")

	result1 := LookupHashRing(ring, key1)
	result2 := LookupHashRing(ring, key1)

	if result1 != result2 {
		t.Errorf("lookup not deterministic: key1 -> %s then %s", result1, result2)
	}

	// Different keys should potentially return different nodes
	result3 := LookupHashRing(ring, key2)
	_ = result3 // Just verify it doesn't panic
}

func TestLookupHashRing_WrapAround(t *testing.T) {
	members := []string{"node-a", "node-b"}
	ring := BuildHashRing(members, 10)

	// Get all hash points and verify wrap-around
	for i := 0; i < 1000; i++ {
		keyHash := uint64(i)
		result := LookupHashRing(ring, keyHash)
		if result != "node-a" && result != "node-b" {
			t.Errorf("unexpected node result: %s", result)
		}
	}
}

func TestSelectTopN(t *testing.T) {
	stats := []nodeStat{
		{tag: "fast", delay: 50, failure: false},
		{tag: "medium", delay: 100, failure: false},
		{tag: "slow", delay: 200, failure: false},
		{tag: "failed", delay: 0, failure: true},
	}

	result := SelectTopN(stats, 2, 0, nil)

	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	if result[0] != "fast" {
		t.Errorf("expected first result to be 'fast', got '%s'", result[0])
	}

	if result[1] != "medium" {
		t.Errorf("expected second result to be 'medium', got '%s'", result[1])
	}
}

func TestSelectTopN_WithTolerance(t *testing.T) {
	stats := []nodeStat{
		{tag: "fast", delay: 50, failure: false},
		{tag: "medium", delay: 100, failure: false},
		{tag: "slow", delay: 150, failure: false},
	}

	// Select with n=2, tolerance=25
	result := SelectTopN(stats, 2, 25, []string{"medium"})

	// cutoffIdx=1, cutoffDelay=100, maxAllowedDelay=125
	// "slow" (150) is outside tolerance, "medium" (100) and "fast" (50) are in
	// Result should be ["fast", "medium"] (sorted by delay)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	if result[0] != "fast" {
		t.Errorf("expected first result to be 'fast', got '%s'", result[0])
	}

	if result[1] != "medium" {
		t.Errorf("expected second result to be 'medium', got '%s'", result[1])
	}
}

func TestSelectTopN_TolerancePreservesPrevious(t *testing.T) {
	stats := []nodeStat{
		{tag: "new-fast", delay: 40, failure: false},
		{tag: "previous-slow", delay: 90, failure: false},
		{tag: "other", delay: 120, failure: false},
	}

	// tolerance=50, n=2
	// cutoffIdx=1, cutoffDelay=90, maxAllowedDelay=140
	// previous-slow (90) is within tolerance of cutoff (90+50=140)
	result := SelectTopN(stats, 2, 50, []string{"previous-slow"})

	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	// "new-fast" (40) is fastest, "previous-slow" (90) is kept due to tolerance
	// Result: ["new-fast", "previous-slow"]
	if result[0] != "new-fast" {
		t.Errorf("expected first result to be 'new-fast', got '%s'", result[0])
	}
	if result[1] != "previous-slow" {
		t.Errorf("expected second result to be 'previous-slow', got '%s'", result[1])
	}
}

func TestSelectTopN_EmptyStats(t *testing.T) {
	stats := []nodeStat{}

	result := SelectTopN(stats, 2, 0, nil)

	if len(result) != 0 {
		t.Errorf("expected 0 results for empty stats, got %d", len(result))
	}
}

func TestSelectTopN_AllFailed(t *testing.T) {
	stats := []nodeStat{
		{tag: "failed1", delay: 0, failure: true},
		{tag: "failed2", delay: 0, failure: true},
	}

	result := SelectTopN(stats, 2, 0, nil)

	if len(result) != 0 {
		t.Errorf("expected 0 results when all failed, got %d", len(result))
	}
}

func TestSelectTopN_NLessThanStats(t *testing.T) {
	stats := []nodeStat{
		{tag: "a", delay: 50, failure: false},
		{tag: "b", delay: 100, failure: false},
		{tag: "c", delay: 150, failure: false},
		{tag: "d", delay: 200, failure: false},
	}

	result := SelectTopN(stats, 2, 0, nil)

	if len(result) != 2 {
		t.Errorf("expected 2 results, got %d", len(result))
	}
}

func TestSelectTopN_NGreaterThanStats(t *testing.T) {
	stats := []nodeStat{
		{tag: "a", delay: 50, failure: false},
		{tag: "b", delay: 100, failure: false},
	}

	result := SelectTopN(stats, 5, 0, nil)

	if len(result) != 2 {
		t.Errorf("expected 2 results (capped by stats length), got %d", len(result))
	}
}

func TestSelectTopN_ZeroN(t *testing.T) {
	stats := []nodeStat{
		{tag: "a", delay: 50, failure: false},
	}

	result := SelectTopN(stats, 0, 0, nil)

	if len(result) != 0 {
		t.Errorf("expected 0 results for n=0, got %d", len(result))
	}
}

func TestSelectTopN_FastPathNoTolerance(t *testing.T) {
	stats := []nodeStat{
		{tag: "fast", delay: 50, failure: false},
		{tag: "slow", delay: 500, failure: false},
	}

	prevCandidates := []string{"slow"} // This should be ignored when tolerance=0

	result := SelectTopN(stats, 1, 0, prevCandidates)

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	// Should return "fast" not "slow" because tolerance=0 disables stabilization
	if result[0] != "fast" {
		t.Errorf("expected 'fast' (lowest delay), got '%s'", result[0])
	}
}

func TestHysteresisState(t *testing.T) {
	state := NewHysteresisState()

	if !IsPrimaryActive(state) {
		t.Error("new state should be primary active")
	}

	if state.activeTier != "primary" {
		t.Errorf("expected 'primary', got '%s'", state.activeTier)
	}

	if state.primaryFailureCount != 0 {
		t.Errorf("expected 0 failure count, got %d", state.primaryFailureCount)
	}
}

func TestApplyHysteresis_PrimaryToBackup(t *testing.T) {
	state := NewHysteresisState()

	// Primary becomes unavailable, backup is available
	// After 3 failures, should switch to backup
	for i := 0; i < 3; i++ {
		state = ApplyHysteresis(state, false, true, 3, time.Second)
	}

	if IsPrimaryActive(state) {
		t.Error("should have switched to backup after 3 failures")
	}

	if state.activeTier != "backup" {
		t.Errorf("expected 'backup', got '%s'", state.activeTier)
	}
}

func TestApplyHysteresis_PrimaryRecovers(t *testing.T) {
	state := NewHysteresisState()

	// Switch to backup
	for i := 0; i < 3; i++ {
		state = ApplyHysteresis(state, false, true, 3, time.Second)
	}

	// Primary becomes available but wait time not elapsed
	state = ApplyHysteresis(state, true, true, 3, time.Second)

	if IsPrimaryActive(state) {
		t.Error("should still be backup before hold time elapses")
	}

	// Simulate time passing (backupHoldTime = 1ms for testing)
	state.backupActivatedAt = time.Now().Add(-time.Second)

	// Now primary should be available again
	state = ApplyHysteresis(state, true, true, 3, time.Millisecond)

	if !IsPrimaryActive(state) {
		t.Error("should have recovered to primary after hold time")
	}
}

func TestApplyHysteresis_PrimaryFailuresReset(t *testing.T) {
	state := NewHysteresisState()

	// Two failures
	state = ApplyHysteresis(state, false, true, 3, time.Second)
	state = ApplyHysteresis(state, false, true, 3, time.Second)

	if state.primaryFailureCount != 2 {
		t.Errorf("expected 2 failure count, got %d", state.primaryFailureCount)
	}

	// Primary becomes available - should reset counter
	state = ApplyHysteresis(state, true, true, 3, time.Second)

	if state.primaryFailureCount != 0 {
		t.Errorf("expected 0 failure count after recovery, got %d", state.primaryFailureCount)
	}
}

func TestApplyHysteresis_BackupUnavailable(t *testing.T) {
	state := NewHysteresisState()

	// Primary fails, backup unavailable - should stay in primary
	for i := 0; i < 3; i++ {
		state = ApplyHysteresis(state, false, false, 3, time.Second)
	}

	if !IsPrimaryActive(state) {
		t.Error("should stay in primary when backup is unavailable")
	}

	if state.primaryFailureCount < 3 {
		t.Errorf("expected failure count >= 3, got %d", state.primaryFailureCount)
	}
}

func TestApplyHysteresis_NilState(t *testing.T) {
	state := ApplyHysteresis(nil, true, true, 3, time.Second)

	if !IsPrimaryActive(state) {
		t.Error("nil state should default to primary")
	}
}

func TestApplyHysteresis_MultipleCycles(t *testing.T) {
	state := NewHysteresisState()

	// Cycle 1: Primary -> Backup -> Primary
	for i := 0; i < 3; i++ {
		state = ApplyHysteresis(state, false, true, 3, time.Millisecond)
	}
	state.backupActivatedAt = time.Now().Add(-time.Second)
	state = ApplyHysteresis(state, true, true, 3, time.Millisecond)

	if !IsPrimaryActive(state) {
		t.Error("should be back to primary")
	}

	// Cycle 2: Primary -> Backup -> Primary
	for i := 0; i < 3; i++ {
		state = ApplyHysteresis(state, false, true, 3, time.Millisecond)
	}
	state.backupActivatedAt = time.Now().Add(-time.Second)
	state = ApplyHysteresis(state, true, true, 3, time.Millisecond)

	if !IsPrimaryActive(state) {
		t.Error("should be back to primary after second cycle")
	}
}

func TestBuildHashKey(t *testing.T) {
	metadata := &C.Metadata{
		SrcIP: netip.MustParseAddr("192.168.1.100"),
		DstIP: netip.MustParseAddr("8.8.8.8"),
		SrcPort: 12345,
		DstPort: 443,
		Host: "example.com",
	}

	keyParts := []string{"src_ip", "dst_ip"}
	key := BuildHashKey(metadata, keyParts, "prod-")

	expected := "prod-192.168.1.100|8.8.8.8"
	if key != expected {
		t.Errorf("expected '%s', got '%s'", expected, key)
	}
}

func TestBuildHashKey_Domain(t *testing.T) {
	metadata := &C.Metadata{
		Host: "api.example.com",
	}

	keyParts := []string{"domain"}
	key := BuildHashKey(metadata, keyParts, "")

	if key != "api.example.com" {
		t.Errorf("expected 'api.example.com', got '%s'", key)
	}
}

func TestBuildHashKey_ETLDPlusOne(t *testing.T) {
	metadata := &C.Metadata{
		Host: "api.example.com",
	}

	keyParts := []string{"etld_plus_one"}
	key := BuildHashKey(metadata, keyParts, "")

	if key != "example.com" {
		t.Errorf("expected 'example.com', got '%s'", key)
	}
}

func TestBuildHashKey_AllParts(t *testing.T) {
	metadata := &C.Metadata{
		SrcIP: netip.MustParseAddr("192.168.1.100"),
		DstIP: netip.MustParseAddr("8.8.8.8"),
		SrcPort: 12345,
		DstPort: 443,
		NetWork: C.TCP,
		Host: "example.com",
		InName: "inbound-tun0",
		DstIPASN: "15169",
	}

	keyParts := []string{"src_ip", "dst_ip", "src_port", "dst_port", "network", "domain", "inbound_tag", "dst_asn"}
	key := BuildHashKey(metadata, keyParts, "")

	expected := "192.168.1.100|8.8.8.8|12345|443|tcp|example.com|inbound-tun0|15169"
	if key != expected {
		t.Errorf("expected '%s', got '%s'", expected, key)
	}
}

func TestBuildHashKey_MatchedRulesetFallback(t *testing.T) {
	metadata := &C.Metadata{
		Host: "api.example.com",
	}

	// matched_ruleset_or_etld should fallback to etld_plus_one in mihomo
	keyParts := []string{"matched_ruleset_or_etld"}
	key := BuildHashKey(metadata, keyParts, "")

	if key != "example.com" {
		t.Errorf("expected 'example.com' (etld+1 fallback), got '%s'", key)
	}
}

func TestBuildHashKey_IPAddressETLD(t *testing.T) {
	metadata := &C.Metadata{
		Host: "192.168.1.1",
	}

	keyParts := []string{"etld_plus_one"}
	key := BuildHashKey(metadata, keyParts, "")

	// IP addresses should return "-" placeholder
	if key != "-" {
		t.Errorf("expected '-' for IP address, got '%s'", key)
	}
}

func TestBuildHashKey_EmptyMetadata(t *testing.T) {
	metadata := &C.Metadata{}

	keyParts := []string{"src_ip", "dst_ip"}
	key := BuildHashKey(metadata, keyParts, "salt-")

	if key != "salt-" {
		t.Errorf("expected 'salt-' (empty parts), got '%s'", key)
	}
}

func TestBuildHashKey_NilMetadata(t *testing.T) {
	keyParts := []string{"src_ip"}
	key := BuildHashKey(nil, keyParts, "salt-")

	if key != "salt-" {
		t.Errorf("expected 'salt-' (nil metadata), got '%s'", key)
	}
}

func TestBuildHashKey_EmptyKeyParts(t *testing.T) {
	metadata := &C.Metadata{
		Host: "example.com",
	}

	key := BuildHashKey(metadata, []string{}, "salt-")

	if key != "salt-" {
		t.Errorf("expected 'salt-' (empty key parts), got '%s'", key)
	}
}

func TestBuildHashKey_UnknownPart(t *testing.T) {
	metadata := &C.Metadata{
		SrcIP: netip.MustParseAddr("192.168.1.100"),
		Host: "example.com",
	}

	// Unknown parts should be ignored
	keyParts := []string{"src_ip", "unknown_part", "domain"}
	key := BuildHashKey(metadata, keyParts, "")

	expected := "192.168.1.100|example.com"
	if key != expected {
		t.Errorf("expected '%s', got '%s'", expected, key)
	}
}

func TestBuildHashKey_Network(t *testing.T) {
	tests := []struct {
		network  C.NetWork
		expected string
	}{
		{C.TCP, "tcp"},
		{C.UDP, "udp"},
		{C.ALLNet, "all"},
		{C.InvalidNet, "invalid"},
	}

	for _, test := range tests {
		metadata := &C.Metadata{
			NetWork: test.network,
		}

		key := BuildHashKey(metadata, []string{"network"}, "")
		if key != test.expected {
			t.Errorf("network %v: expected '%s', got '%s'", test.network, test.expected, key)
		}
	}
}

func TestIsHashKeyPartSupported(t *testing.T) {
	supported := []string{"src_ip", "dst_ip", "src_port", "dst_port", "network", "domain", "inbound_tag", "dst_asn", "etld_plus_one", "matched_ruleset_or_etld"}

	for _, part := range supported {
		if !IsHashKeyPartSupported(part) {
			t.Errorf("expected '%s' to be supported", part)
		}
	}

	unsupported := []string{"dst_geosite", "unknown", "src_asn", "user", "password"}
	for _, part := range unsupported {
		if IsHashKeyPartSupported(part) {
			t.Errorf("expected '%s' to NOT be supported", part)
		}
	}
}

func TestParseHashKeyParts(t *testing.T) {
	input := []string{"src_ip", "dst_geosite", "dst_ip", "unknown", "domain"}
	valid := ParseHashKeyParts(input)

	if len(valid) != 3 {
		t.Errorf("expected 3 valid parts, got %d", len(valid))
	}

	expected := []string{"src_ip", "dst_ip", "domain"}
	for i, part := range valid {
		if part != expected[i] {
			t.Errorf("expected part[%d]='%s', got '%s'", i, expected[i], part)
		}
	}
}

func TestExtractETLDPlusOne(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"www.example.com", "example.com"},
		{"api.example.com", "example.com"},
		{"a.b.example.com", "example.com"},
		{"example.co.uk", "example.co.uk"}, // UK is in PSL
		{"192.168.1.1", "-"},              // IP address
		{"invalid..", "invalid.."},         // Invalid domain falls back to itself
	}

	for _, test := range tests {
		result := extractETLDPlusOne(test.input)
		if result != test.expected {
			t.Errorf("extractETLDPlusOne(%s): expected '%s', got '%s'", test.input, test.expected, result)
		}
	}
}

func TestComputeHashFromKey(t *testing.T) {
	key1 := "test-key-1"
	key2 := "test-key-2"

	hash1a := ComputeHashFromKey(key1)
	hash1b := ComputeHashFromKey(key1)
	hash2 := ComputeHashFromKey(key2)

	// Same key should produce same hash
	if hash1a != hash1b {
		t.Error("same key should produce same hash")
	}

	// Different keys should likely produce different hashes
	if hash1a == hash2 {
		t.Error("different keys should produce different hashes (highly likely)")
	}
}

func TestIsValidMetadataForHash(t *testing.T) {
	validMetadata := &C.Metadata{
		Host: "example.com",
	}

	ipMetadata := &C.Metadata{
		DstIP: netip.MustParseAddr("8.8.8.8"),
	}

	emptyMetadata := &C.Metadata{}

	if !IsValidMetadataForHash(validMetadata) {
		t.Error("metadata with Host should be valid")
	}

	if !IsValidMetadataForHash(ipMetadata) {
		t.Error("metadata with DstIP should be valid")
	}

	if IsValidMetadataForHash(emptyMetadata) {
		t.Error("empty metadata should be invalid")
	}

	if IsValidMetadataForHash(nil) {
		t.Error("nil metadata should be invalid")
	}
}

func TestBuildHashKeyWithOnEmptyKey(t *testing.T) {
	metadata := &C.Metadata{} // Empty metadata

	t.Run("empty_key_random", func(t *testing.T) {
		// For "random", we return empty string and let caller handle
		key, err := BuildHashKeyWithOnEmptyKey(metadata, []string{"src_ip"}, "salt-", "random")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if key != "" {
			t.Errorf("expected empty key for random, got '%s'", key)
		}
	})

	t.Run("empty_key_fail", func(t *testing.T) {
		key, err := BuildHashKeyWithOnEmptyKey(metadata, []string{"src_ip"}, "salt-", "fail")
		if err == nil {
			t.Error("expected error for fail policy")
		}
		if key != "" {
			t.Errorf("expected empty key on error, got '%s'", key)
		}
	})

	t.Run("empty_key_default", func(t *testing.T) {
		key, err := BuildHashKeyWithOnEmptyKey(metadata, []string{"src_ip"}, "salt-", "default")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if key != "salt-" {
			t.Errorf("expected 'salt-' for default policy, got '%s'", key)
		}
	})

	t.Run("non_empty_key", func(t *testing.T) {
		validMetadata := &C.Metadata{
			SrcIP: netip.MustParseAddr("192.168.1.100"),
		}
		key, err := BuildHashKeyWithOnEmptyKey(validMetadata, []string{"src_ip"}, "salt-", "random")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if key != "salt-192.168.1.100" {
			t.Errorf("expected 'salt-192.168.1.100', got '%s'", key)
		}
	})
}

func TestGetMetadataHashParts(t *testing.T) {
	parts := GetMetadataHashParts()

	if len(parts) != 10 {
		t.Errorf("expected 10 supported parts, got %d", len(parts))
	}

	// Verify all expected parts are present
	expectedParts := map[string]bool{
		"src_ip": true, "dst_ip": true, "src_port": true, "dst_port": true,
		"network": true, "domain": true, "inbound_tag": true, "dst_asn": true,
		"etld_plus_one": true, "matched_ruleset_or_etld": true,
	}

	for _, part := range parts {
		if !expectedParts[part] {
			t.Errorf("unexpected hash part: %s", part)
		}
	}
}

func TestHashRing_DeterministicBuild(t *testing.T) {
	members := []string{"a", "b", "c"}

	ring1 := BuildHashRing(members, 50)
	ring2 := BuildHashRing(members, 50)

	// Same inputs should produce same structure
	if HashRingMemberCount(ring1) != HashRingMemberCount(ring2) {
		t.Error("member count should be deterministic")
	}

	if HashRingPointCount(ring1) != HashRingPointCount(ring2) {
		t.Error("point count should be deterministic")
	}

	// Lookup should be identical
	key := ComputeHashFromKey("test-key")
	result1 := LookupHashRing(ring1, key)
	result2 := LookupHashRing(ring2, key)

	if result1 != result2 {
		t.Errorf("lookup should be deterministic: ring1->%s, ring2->%s", result1, result2)
	}
}

func TestSelectTopN_Deterministic(t *testing.T) {
	stats := []nodeStat{
		{tag: "a", delay: 100, failure: false},
		{tag: "b", delay: 50, failure: false},
		{tag: "c", delay: 150, failure: false},
	}

	// Run multiple times with same input, should get same output
	for i := 0; i < 10; i++ {
		result := SelectTopN(stats, 2, 10, []string{"a", "c"})

		if len(result) != 2 {
			t.Errorf("run %d: expected 2 results, got %d", i, len(result))
		}

		// First should always be "b" (lowest delay)
		if result[0] != "b" {
			t.Errorf("run %d: expected first='b', got '%s'", i, result[0])
		}
	}
}