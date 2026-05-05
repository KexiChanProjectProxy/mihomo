# Development

## Requirements

- [Go 1.20+](https://go.dev/dl/)

## Building from Source

```shell
git clone https://github.com/MetaCubeX/mihomo.git
cd mihomo
go mod download
go build
```

## Build with gVisor TUN Stack

```shell
go build -tags with_gvisor
```

## Go Proxy

If GitHub connectivity is limited:

```shell
go env -w GOPROXY=https://goproxy.io,direct
```

## Debugging

Enable debug API as documented in the [wiki](https://wiki.metacubex.one/api/#debug).

## Testing

Run the test suite with Docker:

```shell
make test
```

Benchmark (Linux only):

```shell
make benchmark
```

See [Testing Reference](/reference/testing) for details.