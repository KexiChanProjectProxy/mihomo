# Introduction

Mihomo is a rule-based proxy utility that acts as a kernel compatible with the Clash ecosystem. It provides a local HTTP/HTTPS/SOCKS proxy server with authentication, comprehensive protocol support, and flexible routing capabilities.

## Project Overview

Mihomo supports a wide range of proxy protocols:

- VMess (including AEAD, HTTP, HTTP2, TLS, WebSocket, and gRPC transports)
- VLESS
- Shadowsocks (with ObfsHTTP, ObfsTLS, and V2rayPlugin obfuscation)
- Trojan
- Snell
- TUIC
- Hysteria

The built-in DNS server is designed to minimize DNS pollution attack impact and supports DoH/DoT upstream servers alongside FakeIP mode.

## Dashboard

A dedicated web dashboard with first-class Mihomo support is available at [metacubexd](https://github.com/MetaCubeX/metacubexd). It provides a graphical interface for managing your proxy configuration, viewing connected clients, and monitoring traffic.

## Configuration

A configuration example is provided at [`docs/config.yaml`](https://github.com/MetaCubeX/mihomo/blob/Alpha/docs/config.yaml) in the repository. This covers all the major configuration sections including proxy providers, rule sets, and DNS settings.

## Documentation

Some historical documentation, including detailed API references and legacy guides, continues to live on the [external wiki](https://wiki.metacubex.one/). However, the primary documentation source for this project is the docs site you are currently reading. For up-to-date guides and reference material, explore the navigation sections.

## Next Steps

- Follow the [Development Guide](/guide/development) if you want to build Mihomo from source or contribute to the project.
- See the [Testing Reference](/reference/testing) for information about the protocol test suite and how to run benchmarks.
- Refer to the [Configuration Example](/reference/config-example) for a guided tour of the shipped example configuration.
