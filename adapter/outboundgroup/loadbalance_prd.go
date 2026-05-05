package outboundgroup

import (
	"fmt"
	"net"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/metacubex/mihomo/common/utils"
	"github.com/metacubex/mihomo/constant"

	"golang.org/x/net/publicsuffix"
)

// nodeStat represents a node's statistics for Top-N selection.
type nodeStat struct {
	tag      string
	delay    uint16
	failure  bool
}

// tierStateSnapshot represents the current state of the tier system for hysteresis.
type tierStateSnapshot struct {
	activeTier           string // "primary" or "backup"
	primaryFailureCount  int
	backupActivatedAt    time.Time
}

// HashRing represents a consistent hash ring with virtual nodes.
type HashRing struct {
	points       []uint64      // sorted hash points on the ring
	nodeMap      map[uint64]string // hash point to node tag
	members      []string       // ordered list of member tags
	virtualNodes int            // number of virtual nodes per member
}

// BuildHashRing constructs a consistent hash ring from a list of proxy tags.
// Each member gets `virtualNodes` virtual nodes distributed around the ring.
func BuildHashRing(members []string, virtualNodes int) *HashRing {
	if len(members) == 0 {
		return &HashRing{
			points:       []uint64{},
			nodeMap:      make(map[uint64]string),
			members:      []string{},
			virtualNodes: virtualNodes,
		}
	}

	if virtualNodes < 0 {
		virtualNodes = 0
	}

	ring := &HashRing{
		points:       make([]uint64, 0, len(members)*virtualNodes),
		nodeMap:      make(map[uint64]string),
		members:      members,
		virtualNodes: virtualNodes,
	}

	for i, member := range members {
		ring.members[i] = member

		// Create virtual nodes
		for j := 0; j < virtualNodes; j++ {
			virtualKey := fmt.Sprintf("%s:%d", member, j)
			hash := utils.MapHash(virtualKey)
			ring.points = append(ring.points, hash)
			ring.nodeMap[hash] = member
		}
	}

	// Sort points for binary search
	sort.Slice(ring.points, func(i, j int) bool {
		return ring.points[i] < ring.points[j]
	})

	return ring
}

// LookupHashRing finds the node responsible for the given key hash using binary search.
// Returns empty string if the ring is empty.
func LookupHashRing(ring *HashRing, keyHash uint64) string {
	if len(ring.points) == 0 {
		return ""
	}

	// Binary search for first point >= keyHash
	idx := sort.Search(len(ring.points), func(i int) bool {
		return ring.points[i] >= keyHash
	})

	// Wrap around if necessary
	if idx >= len(ring.points) {
		idx = 0
	}

	return ring.nodeMap[ring.points[idx]]
}

// HashRingMemberCount returns the number of unique members in the ring.
func HashRingMemberCount(ring *HashRing) int {
	return len(ring.members)
}

// HashRingPointCount returns the total number of points (virtual nodes) in the ring.
func HashRingPointCount(ring *HashRing) int {
	return len(ring.points)
}

// SelectTopN selects the top N candidates from the given node stats with tolerance stabilization.
// The tolerance parameter specifies the maximum delay difference allowed for retaining
// previously selected candidates. prevCandidateTags are the tags from the previous selection
// that should be considered for retention if within tolerance.
func SelectTopN(stats []nodeStat, n int, tolerance uint16, prevCandidateTags []string) []string {
	if n <= 0 {
		return []string{}
	}

	// Filter successful nodes
	successNodes := filterSuccessful(stats)
	if len(successNodes) == 0 {
		return []string{}
	}

	// Sort by delay (ascending)
	sort.Slice(successNodes, func(i, j int) bool {
		return successNodes[i].delay < successNodes[j].delay
	})

	// Fast path when tolerance is disabled or no previous candidates
	if tolerance == 0 || len(prevCandidateTags) == 0 {
		return takeFirstN(successNodes, n)
	}

	// Build a map for quick lookup of previous candidates
	successMap := make(map[string]nodeStat)
	for _, stat := range successNodes {
		successMap[stat.tag] = stat
	}

	// Determine cutoff delay
	cutoffIdx := n - 1
	if cutoffIdx >= len(successNodes) {
		cutoffIdx = len(successNodes) - 1
	}
	cutoffDelay := successNodes[cutoffIdx].delay
	maxAllowedDelay := cutoffDelay + tolerance

	// Build eligible set
	eligibleSet := make(map[string]nodeStat)

	// Add all pure Top-N nodes
	for i := 0; i < n && i < len(successNodes); i++ {
		eligibleSet[successNodes[i].tag] = successNodes[i]
	}

	// Add previous candidates within tolerance
	for _, prevTag := range prevCandidateTags {
		if stat, ok := successMap[prevTag]; ok {
			if stat.delay <= maxAllowedDelay {
				eligibleSet[prevTag] = stat
			}
		}
	}

	// Convert to slice and sort
	eligibleNodes := make([]nodeStat, 0, len(eligibleSet))
	for _, stat := range eligibleSet {
		eligibleNodes = append(eligibleNodes, stat)
	}
	sort.Slice(eligibleNodes, func(i, j int) bool {
		return eligibleNodes[i].delay < eligibleNodes[j].delay
	})

	return takeFirstN(eligibleNodes, n)
}

// filterSuccessful removes failed nodes from the stats slice.
func filterSuccessful(stats []nodeStat) []nodeStat {
	result := make([]nodeStat, 0, len(stats))
	for _, stat := range stats {
		if !stat.failure {
			result = append(result, stat)
		}
	}
	return result
}

// takeFirstN returns the first n tags from the sorted stats slice.
func takeFirstN(stats []nodeStat, n int) []string {
	if n >= len(stats) {
		n = len(stats)
	}
	result := make([]string, n)
	for i := 0; i < n; i++ {
		result[i] = stats[i].tag
	}
	return result
}

// NewHysteresisState creates a new hysteresis state with primary tier active.
func NewHysteresisState() *tierStateSnapshot {
	return &tierStateSnapshot{
		activeTier: "primary",
	}
}

// ApplyHysteresis applies hysteresis state transitions based on primary and backup availability.
// This implements HAProxy-style hysteresis behavior:
// - Primary failures accumulate before switching to backup
// - After switching to backup, must wait backupHoldTime before returning to primary
func ApplyHysteresis(current *tierStateSnapshot, primaryAvailable, backupAvailable bool, hystPrimaryFailures int, hystBackupHoldTime time.Duration) *tierStateSnapshot {
	if current == nil {
		current = NewHysteresisState()
	}

	newState := &tierStateSnapshot{
		activeTier:          current.activeTier,
		primaryFailureCount: current.primaryFailureCount,
		backupActivatedAt:   current.backupActivatedAt,
	}

	switch current.activeTier {
	case "primary":
		if !primaryAvailable {
			newState.primaryFailureCount++
			if newState.primaryFailureCount >= hystPrimaryFailures && backupAvailable {
				newState.activeTier = "backup"
				newState.backupActivatedAt = time.Now()
			}
		} else {
			newState.primaryFailureCount = 0
		}
	case "backup":
		if primaryAvailable {
			if time.Since(current.backupActivatedAt) >= hystBackupHoldTime {
				newState.activeTier = "primary"
				newState.primaryFailureCount = 0
			}
		}
	}

	return newState
}

// IsPrimaryActive returns true if the primary tier is currently active.
func IsPrimaryActive(state *tierStateSnapshot) bool {
	if state == nil {
		return true
	}
	return state.activeTier == "primary"
}

// HashKeyPart represents a supported hash key part.
type HashKeyPart string

const (
	HashKeySrcIP            HashKeyPart = "src_ip"
	HashKeyDstIP            HashKeyPart = "dst_ip"
	HashKeySrcPort          HashKeyPart = "src_port"
	HashKeyDstPort          HashKeyPart = "dst_port"
	HashKeyNetwork          HashKeyPart = "network"
	HashKeyDomain           HashKeyPart = "domain"
	HashKeyInboundTag       HashKeyPart = "inbound_tag"
	HashKeyDstASN           HashKeyPart = "dst_asn"
	HashKeyETLDPlusOne      HashKeyPart = "etld_plus_one"
	HashKeyMatchedRuleset  HashKeyPart = "matched_ruleset_or_etld" // mihomo uses etld+one fallback
)

// SupportedHashKeyParts contains all supported hash key parts.
// Note: dst_geosite is NOT supported as mihomo does not have this metadata field.
var SupportedHashKeyParts = []HashKeyPart{
	HashKeySrcIP,
	HashKeyDstIP,
	HashKeySrcPort,
	HashKeyDstPort,
	HashKeyNetwork,
	HashKeyDomain,
	HashKeyInboundTag,
	HashKeyDstASN,
	HashKeyETLDPlusOne,
	HashKeyMatchedRuleset,
}

// IsHashKeyPartSupported returns true if the given key part is supported.
func IsHashKeyPartSupported(part string) bool {
	for _, supported := range SupportedHashKeyParts {
		if string(supported) == part {
			return true
		}
	}
	return false
}

// BuildHashKey constructs a hash key string from metadata based on the specified key parts.
// The key is built as: {salt}{part1}|{part2}|{part3}...
// For parts that cannot be derived, empty string is used.
func BuildHashKey(metadata *constant.Metadata, keyParts []string, keySalt string) string {
	if len(keyParts) == 0 {
		if keySalt == "" {
			return ""
		}
		return keySalt
	}

	var parts []string

	for _, part := range keyParts {
		value := extractHashKeyPart(metadata, part)
		if value != "" {
			parts = append(parts, value)
		}
	}

	key := strings.Join(parts, "|")
	return keySalt + key
}

// extractHashKeyPart extracts a single hash key part from metadata.
func extractHashKeyPart(metadata *constant.Metadata, part string) string {
	switch part {
	case string(HashKeySrcIP):
		if metadata != nil && metadata.SrcIP.IsValid() {
			return metadata.SrcIP.String()
		}
		return ""
	case string(HashKeyDstIP):
		if metadata != nil && metadata.DstIP.IsValid() {
			return metadata.DstIP.String()
		}
		return ""
	case string(HashKeySrcPort):
		if metadata != nil {
			return fmt.Sprintf("%d", metadata.SrcPort)
		}
		return ""
	case string(HashKeyDstPort):
		if metadata != nil {
			return fmt.Sprintf("%d", metadata.DstPort)
		}
		return ""
	case string(HashKeyNetwork):
		if metadata != nil {
			return metadata.NetWork.String()
		}
		return ""
	case string(HashKeyDomain):
		if metadata != nil && metadata.Host != "" {
			return metadata.Host
		}
		return ""
	case string(HashKeyInboundTag):
		if metadata != nil && metadata.InName != "" {
			return metadata.InName
		}
		return ""
	case string(HashKeyDstASN):
		if metadata != nil && metadata.DstIPASN != "" {
			return metadata.DstIPASN
		}
		return ""
	case string(HashKeyETLDPlusOne), string(HashKeyMatchedRuleset):
		// matched_ruleset_or_etld falls back to etld_plus_one in mihomo
		// since mihomo doesn't track matched ruleset in metadata
		if metadata != nil && metadata.Host != "" {
			return extractETLDPlusOne(metadata.Host)
		}
		return ""
	default:
		// Unknown parts are silently ignored
		return ""
	}
}

// extractETLDPlusOne extracts the eTLD+1 from a domain using the public suffix list.
// Returns "-" for IP addresses as a placeholder.
// If extraction fails, returns the original domain as fallback.
func extractETLDPlusOne(rawDomain string) string {
	// Normalize: lowercase, remove port if present
	domain := strings.ToLower(rawDomain)
	if idx := strings.Index(domain, ":"); idx != -1 {
		domain = domain[:idx]
	}

	// Check if it's an IP address
	if net.ParseIP(domain) != nil {
		return "-" // IP address placeholder
	}

	// Use public suffix list to get eTLD+1
	etldPlusOne, err := publicsuffix.EffectiveTLDPlusOne(domain)
	if err != nil {
		// Fallback to original domain if PSL lookup fails
		return domain
	}

	return etldPlusOne
}

// BuildHashKeyWithOnEmptyKey builds a hash key and handles the empty key case
// according to the onEmptyKey policy: "random", "fail", or "default".
func BuildHashKeyWithOnEmptyKey(metadata *constant.Metadata, keyParts []string, keySalt string, onEmptyKey string) (string, error) {
	key := BuildHashKey(metadata, keyParts, keySalt)

	if key == "" || key == keySalt { // empty when only salt with no parts
		switch onEmptyKey {
		case "random":
			// Return a random key - in practice this would use random source
			// For deterministic behavior, we use time-based or return empty for caller to handle
			return "", nil
		case "fail":
			return "", fmt.Errorf("hash key is empty and on_empty_key is set to fail")
		case "default":
			return keySalt, nil
		default:
			// Default behavior: use default (keySalt)
			return keySalt, nil
		}
	}

	return key, nil
}

// ComputeHashFromKey computes a uint64 hash from a key string using maphash.
func ComputeHashFromKey(key string) uint64 {
	return utils.MapHash(key)
}

// IsValidMetadataForHash returns true if the metadata has sufficient information
// for hash key building. Returns false for completely empty metadata.
func IsValidMetadataForHash(metadata *constant.Metadata) bool {
	if metadata == nil {
		return false
	}
	// Check if there's at least something to hash
	return metadata.Host != "" || metadata.DstIP.IsValid() || metadata.SrcIP.IsValid()
}

// GetMetadataHashParts returns a description of supported hash key parts for documentation.
func GetMetadataHashParts() []string {
	result := make([]string, len(SupportedHashKeyParts))
	for i, part := range SupportedHashKeyParts {
		result[i] = string(part)
	}
	return result
}

// ParseHashKeyParts parses and validates a list of key part strings.
// Returns only the parts that are supported.
func ParseHashKeyParts(parts []string) []string {
	var validParts []string
	for _, part := range parts {
		if IsHashKeyPartSupported(part) {
			validParts = append(validParts, part)
		}
	}
	return validParts
}

// AddrPortToString converts an AddrPort to string format "IP:Port".
func AddrPortToString(addrPort netip.AddrPort) string {
	return net.JoinHostPort(addrPort.Addr().String(), fmt.Sprintf("%d", addrPort.Port()))
}