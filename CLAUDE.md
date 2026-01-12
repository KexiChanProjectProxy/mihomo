# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Mihomo is a Go-based proxy kernel supporting multiple protocols (VMess, VLESS, Shadowsocks, Trojan, TUIC, Hysteria, etc.) with rule-based routing, DNS management, and a RESTful API. It requires Go 1.20 or newer.

## Build Commands

```bash
# Basic build (no gvisor TUN stack)
go build

# Build with gvisor TUN stack (recommended for TUN support)
go build -tags with_gvisor

# Platform-specific builds via Makefile
make linux-amd64-v3           # Build for Linux AMD64 v3
make darwin-arm64             # Build for macOS ARM64
make windows-amd64-v3         # Build for Windows AMD64 v3
make all                      # Build all primary platforms
make all-arch                 # Build for all platforms

# Build for Docker
make docker

# Create releases (builds + compress)
make releases
```

Binaries are output to `bin/mihomo-<platform>`.

## Testing

```bash
# Run all tests
go test ./... -v

# Run tests with gvisor tag
go test ./... -v -tags with_gvisor

# Run specific package tests
go test ./rules/... -v
go test ./adapter/... -v
go test ./tunnel/... -v

# Run single test
go test ./test -v -run TestClash
```

Integration tests in `test/` use Docker containers for protocol-specific testing. Some tests may be skipped on macOS (inbound tests removed in CI).

## Linting

```bash
# Run linter (requires golangci-lint installed)
make lint
```

## Architecture

### Core Data Flow

```
Client → Listener → Tunnel → Rule Matcher → Proxy Selector → Proxy Adapter → Dialer → DNS → Remote Server
```

### Key Components

- **`main.go`**: Entry point with CLI parsing. Three modes: normal (run server), test config (`-t`), and subcommands (`convert-ruleset`, `generate`). Handles signal-based config reload (SIGHUP).

- **`hub/executor/`**: Configuration dispatcher and lifecycle coordinator. Parses YAML config and applies updates sequentially: users → proxies → rules → sniffer → hosts → DNS → listeners → TUN → tunnel. Manages state: Suspend → InnerLoading → Running.

- **`hub/route/`**: RESTful API server using Chi router. Endpoints for config, proxies, rules, DNS, connections, traffic stats. Supports HTTP/HTTPS/Unix socket/named pipe with authentication.

- **`tunnel/`**: Core routing engine. Receives connections from listeners via `HandleTCPConn()`/`HandleUDPPacket()`, routes through rule matcher and proxy selector. Manages NAT table for UDP sessions and async packet queue with sequence preservation.

- **`listener/`**: Inbound proxy protocols (HTTP, SOCKS5, Mixed, REDIR, TPROXY, TUN, Shadowsocks, Vmess, TUIC). Per-protocol listener recreation with thread-safe management. Supports authentication, LAN filtering, TFO, MPTCP.

- **`adapter/`**: Proxy abstraction layer:
  - **Outbound adapters**: Protocol implementations (Direct, Reject, Shadowsocks/R, SOCKS5, HTTP/S, Vmess, VLESS, Trojan, Hysteria 1/2, WireGuard, TUIC, AnyTLS, etc.)
  - **Outbound groups**: Routing strategies (Selector, Fallback, URLTest, LoadBalance)
  - **Provider system** (`adapter/provider/`): Dynamic proxy management with health checking, version tracking, subscription parsing. Vehicle types: File, HTTP, Compatible, Inline.
  - **AnyTLS** (`adapter/outbound/anytls.go`): Advanced protocol with session pool management, age-based rotation, and proactive connection maintenance.

- **`rules/`**: Flexible matching system with types: Domain, GEOSITE, GEOIP, IP-CIDR, IP-ASN, Port, Process, Network, UID, Inbound. Supports logic operators (AND, OR, NOT, SUB-RULE). Rule providers support YAML, Text, MRS formats.

- **`dns/`**: Integrated DNS resolver with multiple upstream types (UDP, DoH, DoQ), policy-based routing, fallback chains, EDNS-Client-Subnet, fake IP, and platform integration.

- **`config/`**: YAML configuration parsing. Structure includes general settings, inbound config, proxy definitions, providers, rules, DNS, listeners, TLS, experimental features.

- **`component/`**: Supporting services:
  - **Dialer**: Connection establishment with interface/routing mark binding, TCP concurrency, proxy chaining
  - **Sniffer**: TLS/HTTP SNI extraction for app-layer routing
  - **Geodata**: GeoIP/GeoSite database loading (MMDB, binary)
  - **Process**: Process name/path/UID retrieval for process-based rules
  - **Auth, TLS, Pool, NAT, DHCP, NTP, FAKEIP**: Specialty utilities

- **`constant/`**: Interface definitions, adapter types (22+ protocols), metadata structures, provider abstractions.

### Configuration Flow

```
config.yaml → executor.Parse() → config.Config struct
  ↓
ApplyConfig() updates:
  updateUsers() → updateProxies() → updateRules() → updateDNS() → updateListeners() → updateTun() → tunnel.OnRunning()
```

### Provider System

Proxy/Rule providers load via vehicles (HTTP, File, Inline), cached locally with hash-based update detection. Health check tasks registered per provider. Version increments notify subscribers.

## Important Patterns

- **Provider Pattern**: Extensible adapter/provider system for proxies and rules
- **Middleware Chain**: DNS middleware, rule logic operators
- **Factory Pattern**: Rule parser (`rules/parser.go`), proxy adapter creation
- **Thread-safe Globals**: Package-level maps with RWMutex
- **Context-based Cancellation**: Graceful shutdown across goroutines
- **Channel-based Concurrency**: TCP/UDP queues, packet sender channels

## Development Notes

- Build flags inject version and build time: `-ldflags '-X "github.com/metacubex/mihomo/constant.Version=..." -X "github.com/metacubex/mihomo/constant.BuildTime=..."'`
- Branch-based versioning: Alpha branch → `alpha-<short-hash>`, Beta → `beta-<short-hash>`, tags → tag name
- CGO is disabled (`CGO_ENABLED=0`) for static binaries
- Windows 7/8 support requires Go patches (see `.github/patch/`)
- macOS CI removes inbound tests due to platform restrictions

## Project Structure Reference

```
adapter/          # Proxy protocols and outbound groups
  inbound/        # Inbound connection handling
  outbound/       # Outbound proxy implementations
  outboundgroup/  # Proxy group strategies (fallback, loadbalance, etc.)
  provider/       # Dynamic proxy/rule provider system
common/           # Shared utilities and structures
component/        # Supporting services (dialer, sniffer, geodata, etc.)
config/           # Configuration parsing and structures
constant/         # Constants and interface definitions
dns/              # DNS resolver implementation
hub/              # API server and configuration executor
  executor/       # Config application and lifecycle
  route/          # RESTful API routes
listener/         # Inbound protocol listeners
rules/            # Rule matching system
  logic/          # Logic operators (AND, OR, NOT)
  parser.go       # Rule factory
test/             # Integration tests with Docker
transport/        # Protocol-specific transport implementations
tunnel/           # Core routing engine
```

## Recent Feature Additions (2026-01-12)

### AnyTLS Session Management

Advanced session pool management for the AnyTLS protocol with comprehensive lifecycle control.

**Location**: `transport/anytls/session/client.go`, `adapter/outbound/anytls.go`

**Key Features**:
- **Proactive pool maintenance**: Automatically creates sessions to maintain target pool size (`ensure-idle-session`)
- **Age-based rotation**: Automatic session expiration with jitter to prevent thundering herd (`max-connection-lifetime`, `connection-lifetime-jitter`)
- **Rate-limited creation**: Controls session creation speed to prevent connection storms (`ensure-idle-session-create-rate`)
- **Independent protections**: Separate minimum counts for idle vs age-based cleanup (`min-idle-session`, `min-idle-session-for-age`)
- **Debug logging**: Comprehensive logging for idle, age, and proactive cleanup operations

**Configuration**:
```yaml
experimental:
  anytls-session-management:
    ensure-idle-session: 10              # Proactive pool size
    ensure-idle-session-create-rate: 3   # Max new per cycle
    min-idle-session: 5                  # Idle protection
    min-idle-session-for-age: 2          # Age protection
    max-connection-lifetime: 2h          # Age-based rotation
    connection-lifetime-jitter: 30m      # Randomization
    idle-session-timeout: 10m            # Idle timeout
    idle-session-check-interval: 30s     # Cleanup frequency

proxies:
  - name: "anytls-proxy"
    type: anytls
    server: example.com
    port: 443
    password: "secret"
    session-override:                    # Per-proxy overrides
      ensure-idle-session: 20
      max-connection-lifetime: 3600      # In seconds
```

**Design Decisions**:
- Global config in `experimental` section avoids repetition across subscription proxies
- Per-proxy overrides via `session-override` for fine-grained control
- Backward compatible with legacy per-proxy fields (`idle-session-check-interval`, etc.)
- Priority: Per-proxy override > Legacy fields > Global config

**Documentation**: See `docs/anytls-session-management.md` for comprehensive guide

### Metadata Extensions for Hash Routing

Added `MatchedRuleSet` field to connection metadata to enable ruleset-based load balancing.

**Location**: `constant/metadata.go`, `tunnel/tunnel.go`

**Changes**:
- Added `MatchedRuleSet` field to `Metadata` struct
- Populated during rule matching in `tunnel/match()`
- Available for hash-based routing in LoadBalance groups

**Use Case**: Enable LoadBalance groups to hash by matched ruleset for service-category routing (e.g., streaming, gaming, general traffic to different outbounds)

## License

GPL-3.0. Downstream projects not affiliated with MetaCubeX shall not contain "mihomo" in their names.
