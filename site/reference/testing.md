# Testing Reference

Mihomo ships with a protocol test suite that validates the correctness of its proxy protocol implementations.

## Protocol Test Coverage

The test suite covers the following protocols and configurations:

### Shadowsocks

- Normal
- ObfsHTTP
- ObfsTLS
- ObfsV2rayPlugin

### VMess

- Normal
- AEAD
- HTTP
- HTTP2
- TLS
- WebSocket
- WebSocket TLS
- gRPC

### Trojan

- Normal
- gRPC

### Snell

- Normal
- ObfsHTTP
- ObfsTLS

## Test Types

Each protocol variant is tested with:

- TCP pingpong test
- UDP pingpong test
- TCP large data test
- UDP large data test

## DNS Feature Testing

DNS server functionality is also covered:

- DNS Server
- FakeIP
- Host

## Running Tests

### Prerequisites

Docker is required. The test framework supports both Linux and macOS.

### Full Test Suite

```shell
make test
```

This runs all protocol tests inside Docker containers.

### Benchmarks

```shell
make benchmark
```

Benchmark runs are supported on Linux only. Note that benchmark results cannot represent absolute throughput of a protocol on your machine. They are designed to let you compare the relative throughput of different protocol implementations within Mihomo across machines. You can adjust `chunkSize` in the benchmark configuration to measure the maximum throughput Mihomo achieves on your hardware.

## Continuous Integration

The repository now validates the documentation site separately through `.github/workflows/docs.yml`, which runs `npm ci` and `npm run docs:build` in `site/`. Protocol and benchmark commands remain manual or project-specific workflows outside this docs build check.
