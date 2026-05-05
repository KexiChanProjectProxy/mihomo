# Configuration Example

The complete configuration reference for mihomo lives in [`docs/config.yaml`](https://github.com/MetaCubeX/mihomo/blob/Alpha/docs/config.yaml) in the repository. That file contains every available option with inline comments explaining each one.

This page provides a quick orientation to the major configuration sections.

## Key Sections

### Proxy Server (port bindings)

```yaml
mixed-port: 10801        # combined HTTP/SOCKS5 on one port
# port: 7890            # HTTP only
# socks-port: 7891      # SOCKS5 only
# redir-port: 7892      # transparent proxy (Linux/macOS)
# tproxy-port: 7893     # TProxy (Linux, TCP+UDP)
```

### Network Access

```yaml
allow-lan: true         # permit LAN access to the proxy
bind-address: "*"       # bind to all interfaces; use a specific IP to restrict
lan-allowed-ips:        # whitelist of IP ranges that may connect
  - 0.0.0.0/0
  - ::/0
lan-disallowed-ips:     # blacklist (higher priority than allowed-ips)
  - 192.168.0.3/32
skip-auth-prefixes:     # IPs/CIDRs that bypass authentication
  - 127.0.0.1/8
```

Authentication is optional and is a list of `username:password` pairs under the `authentication` key.

### Rule Mode

```yaml
mode: rule              # rule / global / direct / script (default: rule)
find-process-mode: strict  # always / strict / off
```

In `rule` mode, traffic is routed according to rules defined under the `rules` key. The `find-process-mode` option controls whether mihomo attempts to match traffic to processes on the local system.

### Geodata and Auto-Update

```yaml
geox-url:
  geoip:   "https://fastly.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geoip.dat"
  geosite: "https://fastly.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geosite.dat"
  mmdb:    "https://fastly.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geoip.metadb"

geo-auto-update: false
geo-update-interval: 24
```

Custom URLs can replace the default MaxMind and geosite sources. The `geo-auto-update` flag and `geo-update-interval` control whether mihomo refreshes these files automatically.

### DNS

```yaml
dns:
  enable: true
  listen: 0.0.0.0:53
  enhanced-mode: fake-ip     # fake-ip or redir-host
  fake-ip-range: 198.18.0.1/16
  default-nameserver:
    - 114.114.114.114
    - 8.8.8.8
    - tls://1.12.12.12:853
  nameserver:
    - https://doh.pub/dns-query
    - https://dns.alidns.com/dns-query
  nameserver-policy:
    "geosite:cn,private,apple":
      - https://doh.pub/dns-query
      - https://dns.alidns.com/dns-query
```

When `enhanced-mode` is set to `fake-ip`, mihomo returns a synthetic IP address for each DNS query, preventing DNS pollution. The `fake-ip-filter` list controls which domains bypass fake-ip. The `nameserver-policy` section lets specific domain patterns use different DNS servers.

### Proxies

The `proxies` key lists all outbound proxy nodes. Supported protocol types include:

- `socks5`, `http` — basic SOCKS5 and HTTP upstream proxies
- `ss` — Shadowsocks, including obfs and v2ray-plugin variants
- `vmess` — VMess, supporting TCP, HTTP, gRPC, and WebSocket transports
- `vless` — VLESS, including Reality and XHTTP variants
- `trojan` — Trojan, with gRPC and WebSocket support
- `hysteria` / `hysteria2` — Hysteria protocols
- `tuic` — TUIC (QUIC-based)
- `wireguard` — WireGuard
- `snell` — Snell
- `ssh` — SSH
- `dns` — built-in DNS out (traffic stays internal)

Each proxy entry has a `name` (used by rules and proxy groups) and a `type`. Protocol-specific options follow the `type` field.

### Proxy Groups

```yaml
proxy-groups:
  - name: Proxy
    type: select          # select / url-test / fallback / load-balance / relay
    proxies:
      - ss1
      - ss2
      - auto

  - name: auto
    type: url-test        # picks the node with the lowest latency
    url: "https://cp.cloudflare.com/generate_204"
    interval: 300
    proxies:
      - ss1
      - ss2
```

Proxy groups bundle multiple nodes and define how traffic is routed within the group. `select` lets users pick manually; `url-test` and `fallback` switch automatically based on health-check results; `load-balance` distributes connections across nodes; `relay` chains proxies together.

### Rules

```yaml
rules:
  - DOMAIN-SUFFIX,google.com,Proxy
  - DOMAIN-KEYWORD,facebook,Proxy
  - GEOIP,CN,DIRECT
  - MATCH,Proxy
```

Rules are evaluated top-to-bottom. Common rule types include `DOMAIN`, `DOMAIN-SUFFIX`, `DOMAIN-KEYWORD`, `GEOIP`, `IPCIDR`, and `MATCH` (catch-all). Rules reference proxy groups or `DIRECT` by name.

## Full Reference

Do not copy the entire `docs/config.yaml` into a project. The file is maintained in the repository and changes between releases. Instead, fetch the version that corresponds to the mihomo release you are using:

```shell
# replace Alpha with the tag you are running, e.g. v1.18.0
curl -LO https://raw.githubusercontent.com/MetaCubeX/mihomo/Alpha/docs/config.yaml
```

The file is also served at a stable URL within the VitePress public directory at `/config.yaml` when deployed on the documentation site.
