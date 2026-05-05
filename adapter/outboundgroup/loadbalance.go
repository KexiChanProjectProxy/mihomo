package outboundgroup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/metacubex/mihomo/adapter/provider"
	"github.com/metacubex/mihomo/common/callback"
	"github.com/metacubex/mihomo/common/lru"
	"github.com/metacubex/mihomo/common/singledo"
	N "github.com/metacubex/mihomo/common/net"
	"github.com/metacubex/mihomo/common/utils"
	C "github.com/metacubex/mihomo/constant"
	P "github.com/metacubex/mihomo/constant/provider"

	"golang.org/x/net/publicsuffix"
)

type strategyFn = func(proxies []C.Proxy, metadata *C.Metadata, touch bool) C.Proxy

type LoadBalance struct {
	*GroupBase
	disableUDP     bool
	strategyFn     strategyFn
	testUrl        string
	expectedStatus string
	isPRD          bool
	lbOpt          *LoadBalanceOption
	backupProxies  []C.Proxy

	// PRD-mode state
	hystState             *tierStateSnapshot
	candidateSingle       *singledo.Single[prdSelection]
	prevPrimaryCandidates []string
}

// prdSelection holds the result of PRD-mode selection
type prdSelection struct {
	proxy           C.Proxy
	primaryCandidates []string
	backupCandidates  []string
	activeTier       string
}

var errStrategy = errors.New("unsupported strategy")

func parseStrategy(config map[string]any) string {
	if strategy, ok := config["strategy"].(string); ok {
		return strategy
	}
	return "consistent-hashing"
}

func getKey(metadata *C.Metadata) string {
	if metadata == nil {
		return ""
	}

	if metadata.Host != "" {
		// ip host
		if ip := net.ParseIP(metadata.Host); ip != nil {
			return metadata.Host
		}

		if etld, err := publicsuffix.EffectiveTLDPlusOne(metadata.Host); err == nil {
			return etld
		}
	}

	if !metadata.DstIP.IsValid() {
		return ""
	}

	return metadata.DstIP.String()
}

func getKeyWithSrcAndDst(metadata *C.Metadata) string {
	dst := getKey(metadata)
	src := ""
	if metadata != nil {
		src = metadata.SrcIP.String()
	}

	return fmt.Sprintf("%s%s", src, dst)
}

func jumpHash(key uint64, buckets int32) int32 {
	var b, j int64

	for j < int64(buckets) {
		b = j
		key = key*2862933555777941757 + 1
		j = int64(float64(b+1) * (float64(int64(1)<<31) / float64((key>>33)+1)))
	}

	return int32(b)
}

// DialContext implements C.ProxyAdapter
func (lb *LoadBalance) DialContext(ctx context.Context, metadata *C.Metadata) (c C.Conn, err error) {
	proxy := lb.Unwrap(metadata, true)
	c, err = proxy.DialContext(ctx, metadata)

	healthCheckFn := lb.healthCheck
	if lb.isPRD {
		healthCheckFn = lb.healthCheckPRD
	}

	if err == nil {
		c.AppendToChains(lb)
	} else {
		lb.onDialFailed(proxy.Type(), err, healthCheckFn)
	}

	if N.NeedHandshake(c) {
		c = callback.NewFirstWriteCallBackConn(c, func(err error) {
			if err == nil {
				lb.onDialSuccess()
			} else {
				lb.onDialFailed(proxy.Type(), err, healthCheckFn)
			}
		})
	}

	return
}

// ListenPacketContext implements C.ProxyAdapter
func (lb *LoadBalance) ListenPacketContext(ctx context.Context, metadata *C.Metadata) (pc C.PacketConn, err error) {
	defer func() {
		if err == nil {
			pc.AppendToChains(lb)
		}
	}()

	proxy := lb.Unwrap(metadata, true)
	return proxy.ListenPacketContext(ctx, metadata)
}

// SupportUDP implements C.ProxyAdapter
func (lb *LoadBalance) SupportUDP() bool {
	return !lb.disableUDP
}

// IsL3Protocol implements C.ProxyAdapter
func (lb *LoadBalance) IsL3Protocol(metadata *C.Metadata) bool {
	return lb.Unwrap(metadata, false).IsL3Protocol(metadata)
}

func strategyRoundRobin(url string) strategyFn {
	idx := 0
	idxMutex := sync.Mutex{}
	return func(proxies []C.Proxy, metadata *C.Metadata, touch bool) C.Proxy {
		idxMutex.Lock()
		defer idxMutex.Unlock()

		i := 0
		length := len(proxies)

		if touch {
			defer func() {
				idx = (idx + i) % length
			}()
		}

		for ; i < length; i++ {
			id := (idx + i) % length
			proxy := proxies[id]
			if proxy.AliveForTestUrl(url) {
				i++
				return proxy
			}
		}

		return proxies[0]
	}
}

func strategyConsistentHashing(url string) strategyFn {
	maxRetry := 5
	return func(proxies []C.Proxy, metadata *C.Metadata, touch bool) C.Proxy {
		key := utils.MapHash(getKey(metadata))
		buckets := int32(len(proxies))
		for i := 0; i < maxRetry; i, key = i+1, key+1 {
			idx := jumpHash(key, buckets)
			proxy := proxies[idx]
			if proxy.AliveForTestUrl(url) {
				return proxy
			}
		}

		// when availability is poor, traverse the entire list to get the available nodes
		for _, proxy := range proxies {
			if proxy.AliveForTestUrl(url) {
				return proxy
			}
		}

		return proxies[0]
	}
}

func strategyStickySessions(url string) strategyFn {
	ttl := time.Minute * 10
	maxRetry := 5
	lruCache := lru.New[uint64, int](
		lru.WithAge[uint64, int](int64(ttl.Seconds())),
		lru.WithSize[uint64, int](1000))
	return func(proxies []C.Proxy, metadata *C.Metadata, touch bool) C.Proxy {
		key := utils.MapHash(getKeyWithSrcAndDst(metadata))
		length := len(proxies)
		idx, has := lruCache.Get(key)
		if !has || idx >= length {
			idx = int(jumpHash(key+uint64(time.Now().UnixNano()), int32(length)))
		}

		nowIdx := idx
		for i := 1; i < maxRetry; i++ {
			proxy := proxies[nowIdx]
			if proxy.AliveForTestUrl(url) {
				if !has || nowIdx != idx {
					lruCache.Set(key, nowIdx)
				}

				return proxy
			} else {
				nowIdx = int(jumpHash(key+uint64(time.Now().UnixNano()), int32(length)))
			}
		}

		lruCache.Set(key, 0)
		return proxies[0]
	}
}

// Unwrap implements C.ProxyAdapter
func (lb *LoadBalance) Unwrap(metadata *C.Metadata, touch bool) C.Proxy {
	if lb.isPRD {
		return lb.prdUnwrap(metadata, touch)
	}
	proxies := lb.GetProxies(touch)
	return lb.strategyFn(proxies, metadata, touch)
}

// MarshalJSON implements C.ProxyAdapter
func (lb *LoadBalance) MarshalJSON() ([]byte, error) {
	var all []string
	for _, proxy := range lb.GetProxies(false) {
		all = append(all, proxy.Name())
	}
	return json.Marshal(map[string]any{
		"type":           lb.Type().String(),
		"all":            all,
		"testUrl":        lb.testUrl,
		"expectedStatus": lb.expectedStatus,
		"hidden":         lb.Hidden(),
		"icon":           lb.Icon(),
	})
}

func (lb *LoadBalance) Providers() []P.ProxyProvider {
	return lb.providers
}

func (lb *LoadBalance) Proxies() []C.Proxy {
	return lb.GetProxies(false)
}

func (lb *LoadBalance) Now() string {
	return ""
}

func NewLoadBalance(option *GroupCommonOption, providers []P.ProxyProvider, strategy string) (lb *LoadBalance, err error) {
	var strategyFn strategyFn
	switch strategy {
	case "consistent-hashing":
		strategyFn = strategyConsistentHashing(option.URL)
	case "round-robin":
		strategyFn = strategyRoundRobin(option.URL)
	case "sticky-sessions":
		strategyFn = strategyStickySessions(option.URL)
	default:
		return nil, fmt.Errorf("%w: %s", errStrategy, strategy)
	}
	return &LoadBalance{
		GroupBase: NewGroupBase(GroupBaseOption{
			Name:           option.Name,
			Type:           C.LoadBalance,
			Hidden:         option.Hidden,
			Icon:           option.Icon,
			Filter:         option.Filter,
			ExcludeFilter:  option.ExcludeFilter,
			ExcludeType:    option.ExcludeType,
			TestTimeout:    option.TestTimeout,
			MaxFailedTimes: option.MaxFailedTimes,
			Providers:      providers,
		}),
		strategyFn:     strategyFn,
		disableUDP:     option.DisableUDP,
		testUrl:        option.URL,
		expectedStatus: option.ExpectedStatus,
	}, nil
}

func NewLoadBalancePRD(option *GroupCommonOption, primaryProxies []C.Proxy, backupProxies []C.Proxy, lbOpt *LoadBalanceOption, strategy string) (*LoadBalance, error) {
	testUrl := option.URL
	if testUrl == "" {
		testUrl = C.DefaultTestURL
	}
	expectedStatus := option.ExpectedStatus
	if expectedStatus == "" {
		expectedStatus = "*"
	}

	primaryProvider := createSingleProvider(option.Name+"-primary", primaryProxies, testUrl, option)
	providers := []P.ProxyProvider{primaryProvider}
	if len(backupProxies) > 0 {
		backupProvider := createSingleProvider(option.Name+"-backup", backupProxies, testUrl, option)
		providers = append(providers, backupProvider)
	}

	var strategyFn strategyFn
	switch strategy {
	case "consistent-hashing":
		strategyFn = strategyConsistentHashing(testUrl)
	case "round-robin":
		strategyFn = strategyRoundRobin(testUrl)
	case "sticky-sessions":
		strategyFn = strategyStickySessions(testUrl)
	case "random":
		strategyFn = strategyRandom(testUrl)
	default:
		return nil, fmt.Errorf("%w: %s", errStrategy, strategy)
	}

	return &LoadBalance{
		GroupBase: NewGroupBase(GroupBaseOption{
			Name:           option.Name,
			Type:           C.LoadBalance,
			Hidden:         option.Hidden,
			Icon:           option.Icon,
			Filter:         option.Filter,
			ExcludeFilter:  option.ExcludeFilter,
			ExcludeType:    option.ExcludeType,
			TestTimeout:    option.TestTimeout,
			MaxFailedTimes: option.MaxFailedTimes,
			Providers:      providers,
		}),
		strategyFn:          strategyFn,
		disableUDP:          option.DisableUDP,
		testUrl:             testUrl,
		expectedStatus:      expectedStatus,
		isPRD:               true,
		lbOpt:               lbOpt,
		backupProxies:       backupProxies,
		hystState:           NewHysteresisState(),
		candidateSingle:     singledo.NewSingle[prdSelection](time.Second * 10),
		prevPrimaryCandidates: []string{},
	}, nil
}

func createSingleProvider(name string, proxies []C.Proxy, testUrl string, option *GroupCommonOption) P.ProxyProvider {
	expectedStatus, _ := utils.NewUnsignedRanges[uint16](option.ExpectedStatus)
	hc := provider.NewHealthCheck(proxies, testUrl, uint(option.TestTimeout), uint(option.Interval), option.Lazy, expectedStatus)
	pd, _ := provider.NewCompatibleProvider(name, proxies, hc)
	return pd
}

func strategyRandom(url string) strategyFn {
	return func(proxies []C.Proxy, metadata *C.Metadata, touch bool) C.Proxy {
		if len(proxies) == 0 {
			return nil
		}
		for _, proxy := range proxies {
			if proxy.AliveForTestUrl(url) {
				return proxy
			}
		}
		return proxies[0]
	}
}

func (lb *LoadBalance) prdUnwrap(metadata *C.Metadata, touch bool) C.Proxy {
	elm, _, shared := lb.candidateSingle.Do(func() (prdSelection, error) {
		return lb.doPRDSelection(metadata, touch)
	})
	if shared && touch {
		lb.Touch()
	}
	return elm.proxy
}

func (lb *LoadBalance) doPRDSelection(metadata *C.Metadata, touch bool) (prdSelection, error) {
	if touch {
		lb.Touch()
	}

	primaryProxies := lb.GetProxies(false)
	primaryStats := lb.buildNodeStats(primaryProxies)

	primaryN := 1
	backupN := 0
	if lb.lbOpt != nil && lb.lbOpt.TopN != nil {
		primaryN = lb.lbOpt.TopN.Primary
		if primaryN <= 0 {
			primaryN = 1
		}
		backupN = lb.lbOpt.TopN.Backup
	}

	tolerance := uint16(0)
	if lb.lbOpt != nil && lb.lbOpt.Hash != nil {
		tolerance = uint16(lb.lbOpt.Hash.Tolerance)
	}

	primaryCandidates := SelectTopN(primaryStats, primaryN, tolerance, lb.prevPrimaryCandidates)
	lb.prevPrimaryCandidates = primaryCandidates

	var backupStats []nodeStat
	var backupCandidates []string
	if len(lb.backupProxies) > 0 {
		backupStats = lb.buildNodeStats(lb.backupProxies)
		backupCandidates = SelectTopN(backupStats, backupN, tolerance, nil)
	}

	primaryAvailable := len(primaryCandidates) > 0
	backupAvailable := len(backupCandidates) > 0 && len(lb.backupProxies) > 0

	hystPrimaryFailures := 3
	hystBackupHoldTime := time.Second * 5
	if lb.lbOpt != nil && lb.lbOpt.Hysteresis != nil {
		hystPrimaryFailures = lb.lbOpt.Hysteresis.PrimaryFailures
		if hystPrimaryFailures <= 0 {
			hystPrimaryFailures = 3
		}
		hystBackupHoldTime = lb.lbOpt.Hysteresis.BackupHoldTime
		if hystBackupHoldTime <= 0 {
			hystBackupHoldTime = time.Second * 5
		}
	}

	lb.hystState = ApplyHysteresis(lb.hystState, primaryAvailable, backupAvailable, hystPrimaryFailures, hystBackupHoldTime)

	var selectedProxy C.Proxy
	if IsPrimaryActive(lb.hystState) {
		selectedProxy = lb.selectFromCandidates(primaryProxies, primaryCandidates, metadata)
		if selectedProxy == nil && len(backupCandidates) > 0 {
			selectedProxy = lb.selectFromCandidates(lb.backupProxies, backupCandidates, metadata)
		}
	} else {
		selectedProxy = lb.selectFromCandidates(lb.backupProxies, backupCandidates, metadata)
		if selectedProxy == nil && len(primaryCandidates) > 0 {
			selectedProxy = lb.selectFromCandidates(primaryProxies, primaryCandidates, metadata)
		}
	}

	if selectedProxy == nil {
		if lb.lbOpt != nil && lb.lbOpt.EmptyPoolAction == "error" {
			return prdSelection{
				proxy:             nil,
				primaryCandidates: primaryCandidates,
				backupCandidates:  backupCandidates,
				activeTier:        lb.hystState.activeTier,
			}, nil
		}
		if len(primaryProxies) > 0 {
			selectedProxy = primaryProxies[0]
		} else if len(lb.backupProxies) > 0 {
			selectedProxy = lb.backupProxies[0]
		}
	}

	return prdSelection{
		proxy:             selectedProxy,
		primaryCandidates: primaryCandidates,
		backupCandidates:  backupCandidates,
		activeTier:        lb.hystState.activeTier,
	}, nil
}

func (lb *LoadBalance) buildNodeStats(proxies []C.Proxy) []nodeStat {
	stats := make([]nodeStat, 0, len(proxies))
	for _, proxy := range proxies {
		alive := proxy.AliveForTestUrl(lb.testUrl)
		delay := proxy.LastDelayForTestUrl(lb.testUrl)
		stats = append(stats, nodeStat{
			tag:     proxy.Name(),
			delay:   delay,
			failure: !alive,
		})
	}
	return stats
}

func (lb *LoadBalance) selectFromCandidates(allProxies []C.Proxy, candidateTags []string, metadata *C.Metadata) C.Proxy {
	if len(candidateTags) == 0 {
		return nil
	}

	proxyMap := make(map[string]C.Proxy)
	for _, p := range allProxies {
		proxyMap[p.Name()] = p
	}

	var candidates []C.Proxy
	for _, tag := range candidateTags {
		if p, ok := proxyMap[tag]; ok {
			candidates = append(candidates, p)
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	switch lb.lbOpt.Strategy {
	case "consistent-hashing", "consistent_hash":
		return lb.prdStrategyConsistentHashing(candidates, metadata)
	case "round-robin":
		return lb.prdStrategyRoundRobin(candidates, metadata)
	case "sticky-sessions":
		return lb.prdStrategyStickySessions(candidates, metadata)
	case "random":
		return lb.prdStrategyRandom(candidates, metadata)
	default:
		return candidates[0]
	}
}

func (lb *LoadBalance) prdStrategyConsistentHashing(candidates []C.Proxy, metadata *C.Metadata) C.Proxy {
	keyParts := []string{"src_ip", "dst_ip", "dst_port"}
	keySalt := "lb-prd-"
	onEmptyKey := "random"
	hashOpt := lb.lbOpt.Hash
	if hashOpt != nil {
		if len(hashOpt.KeyParts) > 0 {
			keyParts = hashOpt.KeyParts
		}
		if hashOpt.KeySalt != "" {
			keySalt = hashOpt.KeySalt
		}
		if hashOpt.OnEmptyKey != "" {
			onEmptyKey = hashOpt.OnEmptyKey
		}
	}

	if lb.lbOpt != nil && lb.lbOpt.PreferDomain && metadata != nil && metadata.Host != "" {
		newKeyParts := make([]string, 0, len(keyParts))
		for _, part := range keyParts {
			if part == "dst_ip" {
				newKeyParts = append(newKeyParts, "domain")
			} else {
				newKeyParts = append(newKeyParts, part)
			}
		}
		keyParts = newKeyParts
	}

	key, err := BuildHashKeyWithOnEmptyKey(metadata, keyParts, keySalt, onEmptyKey)
	if err != nil {
		return candidates[0]
	}
	if key == "" {
		if onEmptyKey == "random" {
			return candidates[time.Now().UnixNano()%int64(len(candidates))]
		}
		return candidates[0]
	}

	keyHash := ComputeHashFromKey(key)
	members := make([]string, len(candidates))
	proxyByName := make(map[string]C.Proxy)
	for i, p := range candidates {
		members[i] = p.Name()
		proxyByName[p.Name()] = p
	}

	ring := BuildHashRing(members, 100)
	selectedTag := LookupHashRing(ring, keyHash)
	if selectedTag == "" {
		return candidates[0]
	}
	if p, ok := proxyByName[selectedTag]; ok {
		return p
	}
	return candidates[0]
}

func (lb *LoadBalance) prdStrategyRoundRobin(candidates []C.Proxy, metadata *C.Metadata) C.Proxy {
	if len(candidates) == 0 {
		return nil
	}
	for _, p := range candidates {
		if p.AliveForTestUrl(lb.testUrl) {
			return p
		}
	}
	return candidates[0]
}

func (lb *LoadBalance) prdStrategyStickySessions(candidates []C.Proxy, metadata *C.Metadata) C.Proxy {
	key := getKeyWithSrcAndDst(metadata)
	if key == "" {
		return candidates[0]
	}
	keyHash := utils.MapHash(key)
	idx := int(keyHash) % len(candidates)
	return candidates[idx]
}

func (lb *LoadBalance) prdStrategyRandom(candidates []C.Proxy, metadata *C.Metadata) C.Proxy {
	if len(candidates) == 0 {
		return nil
	}
	for _, p := range candidates {
		if p.AliveForTestUrl(lb.testUrl) {
			return p
		}
	}
	return candidates[int(time.Now().UnixNano())%len(candidates)]
}

func (lb *LoadBalance) healthCheckPRD() {
	lb.candidateSingle.Reset()
	lb.GroupBase.healthCheck()
	lb.candidateSingle.Reset()
}
