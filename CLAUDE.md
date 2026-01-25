# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

MCPE Server Proxy - A high-performance UDP proxy for Minecraft Bedrock Edition game servers. Routes MCPE traffic through upstream proxy nodes (Shadowsocks, VMess, Hysteria2) with load balancing, player tracking, and access control.

## Build Commands

```bash
# Full build (frontend + Go binary for Windows/Linux)
build.bat

# Manual Go build (requires frontend built first)
go build -tags=with_utls -ldflags="-s -w" -o mcpeserverproxy.exe cmd/mcpeserverproxy/main.go

# Build frontend only
cd web && npm install && npm run build

# Run tests
go test ./...

# Run tests for specific package
go test ./internal/proxy/...
go test ./internal/config/...
```

## Running

```bash
# Default (uses config.json, server_list.json)
./mcpeserverproxy.exe

# Custom config
./mcpeserverproxy.exe -config myconfig.json -servers myservers.json

# Debug mode
./mcpeserverproxy.exe -debug
```

## Architecture

### Entry Point
`cmd/mcpeserverproxy/main.go` - Initializes all components: config loading, database, proxy server, ACL manager, API server.

### Core Packages (`internal/`)

| Package | Purpose |
|---------|---------|
| `proxy/` | Core proxy logic - listeners, forwarders, outbound management, load balancing |
| `config/` | Configuration management with hot reload (file watching) |
| `api/` | REST API (Gin) + embedded Vue.js dashboard |
| `session/` | Player session tracking with garbage collection |
| `db/` | SQLite persistence (players, sessions, ACL, API keys) |
| `acl/` | Blacklist/whitelist access control |
| `protocol/` | RakNet/MCBE protocol handling, login packet parsing |
| `auth/` | Xbox Live authentication |
| `monitor/` | Prometheus metrics, system stats, goroutine tracking |
| `logger/` | Structured logging with file rotation |

### Proxy Modes
- `passthrough` - Forwards raw RakNet bytes, extracts player info from login packets
- `raknet` - Full RakNet protocol proxy
- `mitm` - Man-in-the-middle with gophertunnel (full protocol access)
- `raw_udp` - Raw UDP forwarding
- `transparent` - Transparent proxy mode

### Key Data Flow
1. Minecraft clients connect via RakNet UDP
2. Proxy extracts player info from login packets
3. Traffic routed through upstream proxy nodes (sing-box dialer factory)
4. Load balancer selects nodes (least-latency, round-robin, random, least-connections)
5. Sessions tracked in memory + persisted to SQLite

### Configuration Files
- `config.json` - Global settings (API port, database path, logging)
- `server_list.json` - MCPE server configurations (targets, ports, proxy modes)
- `proxy_outbounds.json` - Upstream proxy nodes (SS, VMess, Hysteria2)

### Frontend
`web/` - Vue 3 + Vite + Naive UI dashboard, built output goes to `internal/api/dist/` and is embedded in the Go binary.

### Key Dependencies
- `github.com/sandertv/gophertunnel` - MCBE protocol
- `github.com/sandertv/go-raknet` - RakNet UDP protocol
- `github.com/gin-gonic/gin` - HTTP API
- `github.com/apernet/hysteria` - Hysteria2 proxy
- `github.com/sagernet/sing-*` - sing-box proxy protocols
- `modernc.org/sqlite` - Pure Go SQLite
