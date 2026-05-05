---
layout: home

hero:
  name: "Mihomo"
  text: "Another Mihomo Kernel"
  tagline: A rule-based proxy utility with comprehensive protocol support and flexible routing capabilities.
  actions:
    - theme: brand
      text: Get Started
      link: /guide/introduction
    - theme: alt
      text: Development Guide
      link: /guide/development
    - theme: alt
      text: View on GitHub
      link: https://github.com/MetaCubeX/mihomo

features:
  - title: Multi-Protocol Support
    details: Full support for VMess, VLESS, Shadowsocks, Trojan, Snell, TUIC, and Hysteria protocols with robust implementations.
  - title: Advanced DNS
    details: Built-in DNS server designed to minimize DNS pollution impact, with DoH/DoT upstream support and FakeIP mode.
  - title: Flexible Routing
    details: Rule-based forwarding using domains, GEOIP, IPCIDR, or process detection. Remote groups enable automatic fallback, load balancing, and latency-based selection.
  - title: Remote Providers
    details: Fetch node lists remotely instead of hard-coding them. Keep your configuration clean and maintainable.
  - title: Gateway Deployment
    details: Netfilter TCP redirecting support lets you deploy Mihomo as an Internet gateway using iptables on Linux.
  - title: RESTful API
    details: Comprehensive HTTP API for runtime control, compatible with the clash ecosystem.
---