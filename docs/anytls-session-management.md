# AnyTLS Session Management

## Overview

Mihomo's AnyTLS protocol now supports advanced session pool management with fine-grained lifecycle control. This feature allows maintaining warm connection pools for reduced latency, automatic connection rotation for security, and intelligent pool sizing.

## Features

### 1. Proactive Pool Maintenance

**Configuration**: `ensure-idle-session`

Automatically maintains a target pool size by creating new sessions proactively when the pool drops below the threshold.

**Benefits**:
- Pre-warmed connections for reduced latency
- High availability - always ready connections
- Gradual pool recovery after network disruptions

**Use Case**: High-traffic services requiring consistently low latency.

---

### 2. Rate-Limited Pool Creation

**Configuration**: `ensure-idle-session-create-rate`

Limits the number of new sessions created per cleanup cycle to prevent connection storms.

**Values**:
- `0` (default): Unlimited creation
- `1-3`: Slow, gradual recovery (rate-limited destinations)
- `3-5`: Moderate recovery (balanced approach)
- `5-10`: Fast recovery (high-capacity services)

**Use Case**: Prevents triggering rate limits on destination servers during pool recovery.

---

### 3. Age-Based Connection Rotation

**Configuration**: `max-connection-lifetime`

Automatically closes sessions that exceed a maximum age, ensuring regular connection refresh.

**Benefits**:
- Security: Limits connection exposure window
- Load balancing: Distributes traffic across dynamic backend servers
- NAT resilience: Periodic session renewal prevents stale NAT entries
- Prevents indefinite connection lifetimes

**Use Case**: Security-sensitive services or deployments behind dynamic load balancers.

---

### 4. Lifetime Randomization

**Configuration**: `connection-lifetime-jitter`

Adds randomization to connection lifetimes to prevent thundering herd problems.

**How it works**: Each session gets a random lifetime within the range:
```
actual_lifetime = max_connection_lifetime ± connection_lifetime_jitter
```

**Example**:
- Base: 1 hour
- Jitter: 15 minutes
- Result: Sessions live 45-75 minutes

**Benefits**:
- Prevents simultaneous reconnection spikes
- Smooth connection rotation over time
- Reduces load spikes on destination servers

---

### 5. Independent Age Protection

**Configuration**: `min-idle-session-for-age`

Separate minimum session count for age-based cleanup, independent from idle timeout protection.

**Use Cases**:
- **Aggressive age rotation + generous idle protection**:
  - `min-idle-session-for-age: 2` (allow old sessions to rotate)
  - `min-idle-session: 10` (keep idle pool large)

- **Conservative age rotation + aggressive idle cleanup**:
  - `min-idle-session-for-age: 10` (protect many sessions from age rotation)
  - `min-idle-session: 2` (aggressively clean idle sessions)

---

### 6. Idle Cleanup with Protection

**Configuration**: `min-idle-session`, `idle-session-timeout`

Removes sessions idle for longer than the timeout, while protecting a minimum number.

**Debug Logging**: Comprehensive logging shows:
- Sessions found, closed, and protected
- Per-session idle duration
- Summary statistics

---

## Configuration

### Global Configuration (Experimental Section)

All AnyTLS proxies inherit these settings by default:

```yaml
experimental:
  anytls-session-management:
    # Proactive pool maintenance
    ensure-idle-session: 10              # Target pool size
    ensure-idle-session-create-rate: 3   # Max new sessions per cycle

    # Passive protections
    min-idle-session: 5                  # Idle timeout protection
    min-idle-session-for-age: 2          # Age-based protection

    # Age-based rotation
    max-connection-lifetime: 2h          # Maximum session age
    connection-lifetime-jitter: 30m      # Randomization range

    # Idle cleanup
    idle-session-timeout: 10m            # Idle timeout
    idle-session-check-interval: 30s     # Cleanup cycle frequency
```

### Per-Proxy Overrides

Override global settings for specific proxies:

```yaml
proxies:
  - name: "high-availability-proxy"
    type: anytls
    server: example.com
    port: 443
    password: "secret"
    session-override:
      ensure-idle-session: 20            # Override: larger pool
      max-connection-lifetime: 3600      # Override: 1 hour (in seconds)
      connection-lifetime-jitter: 600    # Override: 10 minutes

  - name: "rate-limited-proxy"
    type: anytls
    server: limited.example.com
    port: 443
    password: "secret"
    session-override:
      ensure-idle-session: 5
      ensure-idle-session-create-rate: 1 # Very slow recovery
```

### Backward Compatibility

Legacy per-proxy fields still work:

```yaml
proxies:
  - name: "legacy-proxy"
    type: anytls
    server: example.com
    port: 443
    password: "secret"
    idle-session-check-interval: 30      # Legacy field (seconds)
    idle-session-timeout: 600            # Legacy field (seconds)
    min-idle-session: 5                  # Legacy field
```

**Priority Order**:
1. Per-proxy `session-override` (highest)
2. Legacy per-proxy fields
3. Global `experimental.anytls-session-management`

---

## Configuration Examples

### Example 1: High Availability Service

**Goal**: Always-ready connection pool with periodic rotation

```yaml
experimental:
  anytls-session-management:
    ensure-idle-session: 10
    min-idle-session: 5
    max-connection-lifetime: 2h
    connection-lifetime-jitter: 30m
    idle-session-timeout: 10m
    idle-session-check-interval: 30s

proxies:
  - name: "ha-service"
    type: anytls
    server: ha.example.com
    port: 443
    password: "secret"
```

**Result**:
- Pool maintains 10 ready sessions
- Sessions rotate every 1.5-2.5 hours
- Minimum 5 sessions protected from idle cleanup

---

### Example 2: Rate-Limited Destination

**Goal**: Gradual pool recovery to avoid triggering rate limits

```yaml
experimental:
  anytls-session-management:
    ensure-idle-session: 5
    ensure-idle-session-create-rate: 1   # Only 1 new session per cycle
    idle-session-check-interval: 60s     # Check every minute

proxies:
  - name: "rate-limited"
    type: anytls
    server: limited.example.com
    port: 443
    password: "secret"
```

**Result**:
- Pool recovers at 1 session/minute
- Takes 5 minutes to reach target of 5 sessions
- Prevents rate limit triggers

---

### Example 3: Security-Sensitive Connection

**Goal**: Regular connection rotation with minimal exposure

```yaml
experimental:
  anytls-session-management:
    max-connection-lifetime: 30m         # Rotate every 30 minutes
    connection-lifetime-jitter: 10m      # 20-40 minute range
    min-idle-session-for-age: 2          # Keep minimum 2 sessions
    idle-session-timeout: 5m

proxies:
  - name: "secure-service"
    type: anytls
    server: secure.example.com
    port: 443
    password: "secret"
```

**Result**:
- Sessions live 20-40 minutes
- Automatic rotation distributes connections
- Minimum 2 sessions maintained

---

### Example 4: Subscription with Mixed Requirements

**Goal**: Global defaults with per-proxy overrides

```yaml
experimental:
  anytls-session-management:
    # Conservative defaults for all proxies
    ensure-idle-session: 5
    max-connection-lifetime: 2h
    idle-session-timeout: 10m

proxies:
  # Standard proxy - uses global config
  - name: "standard-1"
    type: anytls
    server: node1.example.com
    port: 443
    password: "secret"

  # High-performance proxy - override for larger pool
  - name: "premium-node"
    type: anytls
    server: premium.example.com
    port: 443
    password: "secret"
    session-override:
      ensure-idle-session: 20
      ensure-idle-session-create-rate: 5

  # Rate-limited proxy - override for slow recovery
  - name: "limited-node"
    type: anytls
    server: limited.example.com
    port: 443
    password: "secret"
    session-override:
      ensure-idle-session: 3
      ensure-idle-session-create-rate: 1
```

---

## Debug Logging

Enable debug logging to monitor session pool behavior:

```bash
# Set log level to debug in config
log-level: debug
```

**Idle Cleanup Logs**:
```
[DEBUG] [AnyTLS] Idle cleanup: found 12 idle sessions, closing 7 (keeping 5 protected)
```

**Age Cleanup Logs**:
```
[DEBUG] [AnyTLS] Age cleanup: closing 3 aged sessions (keeping 2 protected)
```

**Proactive Creation Logs**:
```
[DEBUG] [AnyTLS] Proactive pool maintenance: current=3, target=10, creating 3 sessions
[DEBUG] [AnyTLS] Created proactive session #145
[DEBUG] [AnyTLS] Created proactive session #146
[DEBUG] [AnyTLS] Created proactive session #147
```

---

## How Features Work Together

### Automatic Rotation

Combining `ensure-idle-session` with `max-connection-lifetime` creates automatic rotation:

1. Sessions age and get closed by age-based cleanup
2. Pool drops below `ensure-idle-session` target
3. Proactive maintenance creates new sessions
4. Result: Continuous rotation without service interruption

### Thundering Herd Prevention

`connection-lifetime-jitter` distributes session expiration across time:

1. Without jitter: All sessions created together expire together
2. With jitter: Expirations spread across time window
3. Proactive creation smoothly replaces expiring sessions
4. Result: No traffic spikes

### Rate Limit Protection

`ensure-idle-session-create-rate` prevents connection storms:

1. Network disruption closes all sessions
2. Pool needs recovery from 0 to target
3. Rate limiting spreads creation over multiple cycles
4. Result: Gradual recovery without triggering rate limits

---

## Best Practices

### 1. Start Conservative

Begin with conservative settings and adjust based on monitoring:

```yaml
experimental:
  anytls-session-management:
    ensure-idle-session: 5
    max-connection-lifetime: 2h
    connection-lifetime-jitter: 30m
```

### 2. Match to Traffic Patterns

- **Bursty traffic**: Higher `ensure-idle-session`
- **Steady traffic**: Lower `ensure-idle-session`, faster rotation
- **Low traffic**: Rely on on-demand creation, set `ensure-idle-session: 0`

### 3. Consider Destination Limits

- **Rate-limited APIs**: Low `ensure-idle-session-create-rate`
- **High-capacity services**: High `ensure-idle-session-create-rate`

### 4. Balance Security and Performance

- **High security**: Short `max-connection-lifetime` (15-30 minutes)
- **High performance**: Longer lifetime (1-4 hours)
- Always use jitter for any rotation

### 5. Monitor and Adjust

Watch debug logs to tune settings:
- Frequent "no sessions available" → Increase `ensure-idle-session`
- Many idle closures → Decrease `idle-session-timeout`
- Age closures too frequent → Increase `max-connection-lifetime`

---

## Troubleshooting

### Issue: Frequent "no sessions available" errors

**Symptoms**: Connections fail or have high latency

**Diagnosis**: Pool size insufficient for traffic

**Solution**:
```yaml
experimental:
  anytls-session-management:
    ensure-idle-session: 15              # Increase target
    ensure-idle-session-create-rate: 5   # Faster recovery
```

---

### Issue: Too many connections to destination

**Symptoms**: Destination server rejects connections

**Diagnosis**: Pool size too large or recovery too fast

**Solution**:
```yaml
experimental:
  anytls-session-management:
    ensure-idle-session: 5               # Reduce target
    ensure-idle-session-create-rate: 1   # Slow down recovery
```

---

### Issue: Periodic latency spikes

**Symptoms**: Regular latency spikes at intervals

**Diagnosis**: Thundering herd - all sessions rotating simultaneously

**Solution**:
```yaml
experimental:
  anytls-session-management:
    connection-lifetime-jitter: 30m      # Add jitter
```

---

### Issue: Sessions not rotating

**Symptoms**: Same sessions persist indefinitely

**Diagnosis**: Age-based rotation not configured

**Solution**:
```yaml
experimental:
  anytls-session-management:
    max-connection-lifetime: 2h          # Enable rotation
    connection-lifetime-jitter: 30m
```

---

## Technical Details

### Session Lifecycle

1. **Creation**: Session created on-demand or proactively
2. **Active**: Session in use by streams
3. **Idle**: Session has no active streams, enters idle pool
4. **Cleanup**: Periodic cleanup checks idle and age conditions
5. **Closure**: Session closed if exceeds idle timeout or age

### Cleanup Cycle

Every `idle-session-check-interval` (default 30s):

1. **Idle Check**: Close sessions idle > `idle-session-timeout`
   - Protect minimum `min-idle-session` sessions

2. **Age Check**: Close sessions older than `max-connection-lifetime` + jitter
   - Protect minimum `min-idle-session-for-age` sessions

3. **Proactive Creation**: If pool < `ensure-idle-session`
   - Create up to `ensure-idle-session-create-rate` new sessions
   - Async creation doesn't block cleanup

### Thread Safety

All operations are thread-safe:
- Session pool access protected by mutex
- Atomic counters for session IDs
- Concurrent cleanup and creation operations safe

---

## Comparison with Other Protocols

### AnyTLS vs Standard Protocols

| Feature | AnyTLS | VMess/VLESS | Trojan |
|---------|--------|-------------|---------|
| Session pooling | ✅ | ❌ | ❌ |
| Age-based rotation | ✅ | ❌ | ❌ |
| Proactive creation | ✅ | ❌ | ❌ |
| Connection reuse | ✅ | Limited | Limited |

### Why Use AnyTLS?

- **Lower latency**: Pre-warmed connections
- **Better security**: Regular rotation
- **Higher availability**: Always-ready pool
- **Resilient**: Automatic recovery

---

## Migration Guide

### From Legacy AnyTLS Config

**Before**:
```yaml
proxies:
  - name: "proxy1"
    type: anytls
    server: example.com
    port: 443
    password: "secret"
    idle-session-check-interval: 30
    idle-session-timeout: 600
    min-idle-session: 5
```

**After (Global)**:
```yaml
experimental:
  anytls-session-management:
    idle-session-check-interval: 30s
    idle-session-timeout: 10m
    min-idle-session: 5
    # Add new features
    ensure-idle-session: 10
    max-connection-lifetime: 2h

proxies:
  - name: "proxy1"
    type: anytls
    server: example.com
    port: 443
    password: "secret"
    # Legacy fields removed, uses global config
```

### From Other Protocols

Consider AnyTLS for services requiring:
- Consistently low latency
- High connection volume
- Regular connection rotation
- Resilience to network disruptions

---

## References

- [AnyTLS Protocol Specification](https://github.com/MetaCubeX/mihomo/tree/Alpha/transport/anytls)
- [Mihomo Configuration Guide](https://wiki.metacubex.one/)
- [Session Management Implementation](transport/anytls/session/client.go)

---

**Version**: Mihomo Alpha (2026-01-12)
**Status**: Production Ready
