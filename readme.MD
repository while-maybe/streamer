# GoStream Media Server

A lightweight, DLNA/UPnP 1.5 compliant media server written in Go. It streams local video files to Smart TVs, game consoles, and media players.

This project demonstrates **production-grade application architecture** in Go. It features dependency injection, structured logging, safe concurrency patterns, and secure filesystem handling using the modern standard library.

**Status:** Core functionality complete.

## Technical Highlights

This project serves as a demonstration of robust systems programming in Go:

*   **Concurrency Patterns:** Implements a priority-based **Shutdown Monitor** (`cmd/server/shutdown.go`) that coordinates OS signals (`SIGTERM`), inactivity timers, and hard deadlines using context propagation and channel orchestration.
*   **Modern Standard Library:** Leverages Go 1.25+ features, specifically `os.OpenInRoot`, to create a kernel-level filesystem jail that strictly prevents path traversal attacks.
*   **Embedded Assets:** Uses `embed.FS` to package XML (SOAP/UPnP) and HTML templates directly into the binary, ensuring a single-file deployment while maintaining clean separation between logic and presentation.
*   **Network Programming:** Implements a pure UDP Multicast (SSDP) discovery layer without external dependencies, handling "ByeBye" packets and socket lifecycle to prevent resource leaks.
*   **Systems Awareness:** Includes architecture-aware configuration validation (e.g., bitwise checks to prevent integer overflows on 32-bit vs 64-bit systems).
*   **Zero Dependencies:** The core logic relies almost exclusively on the standard library (using `gofrs/uuid` only for ID generation).
*   **Enterprise Observability:** Implements the Middleware Pattern to capture RED metrics (Rate, Errors, Duration) via Prometheus. Tracks business-critical state (Active Streams) using gauges, enabling real-time load monitoring.
*   **Scalable Storage Architecture:** Supports Multi-Volume mounting (e.g., streaming from multiple disks simultaneously). Each volume operates with independent I/O scheduling.
*   **Resource Protection:** Implements a Semaphore-based I/O Limiter to prevent disk thrashing on low-power hardware (Raspberry Pi/SD Cards) by strictly capping concurrent file reads.

## Requirements

*   **Go 1.25+** (Required for the `os.OpenInRoot` API).
*   Network access to the local LAN (UDP Multicast required for SSDP discovery).

## Installation

```bash
# Clone the repository
git clone https://github.com/yourusername/streamer.git
cd streamer

# Build the binary
go build -o streamer ./cmd/server
```

## Usage
By default, the server streams the current directory on port 8081.

```bash
./streamer [flags] [path_to_videos]
```

## Monitoring
A Dockerized observability stack (Prometheus + Grafana) is included to visualize runtime performance.


### Start the monitoring infrastructure
```bash
docker-compose up -d
```
Metrics Endpoint: `http://localhost:8081/metrics`
Grafana Dashboard: `http://localhost:3000` (User/Pass: admin)
Setup: Import the dashboard definition from `observability/dashboard.json`
### Quick Examples

**Basic:** Stream the user's video folder.
```bash
./streamer /home/user/Videos
```

**Production-like:** Bind to a specific LAN IP, set a custom name for the TV, allocate a 20MB read buffer, and auto-shutdown at 11 PM.
```bash
./streamer \
  ./streamer \
  -http.addr 192.168.1.50:8081 \
  -media.friendlyName "Living Room Server" \
  -media.maxIO 4 \
  -shutdown.at 23:00 \
  /mnt/external_hdd/movies /mnt/sdcard/tv_shows
```

## Configuration

The application uses a strict configuration validation phase before startup (`internal/config`).

### Network & Media
| Flag | Default | Description |
| :--- | :--- | :--- |
| `-http.addr` | `:8081` | TCP address to listen on. Use `IP:PORT` to bind to specific interface. |
| `-media.friendlyName` | `GoStream Server` | Name displayed on client devices (TVs). Max 64 chars. |
| `-media.uuid` | *(Random)* | Unique Device Identifier. Persist this string to maintain device history/identity on clients. |
| `-media.mode` | `buffered` | File access mode. `direct` (OS page cache) or `buffered` (Application RAM buffer). |
| `-media.bufferSize` | `10MB` | Read buffer size. Supports units: B, KB, MB, GB. |
| `-media.maxIO` | `10`	| Max concurrent disk reads per volume. Lower this for SD cards/HDDs to prevent buffering. |

### Lifecycle & Shutdown
The server monitors three distinct shutdown triggers. Whichever happens first terminates the application.

| Flag | Default | Description |
| :--- | :--- | :--- |
| `-shutdown.inactive` | `30m` | Auto-shutdown after duration of no HTTP requests. |
| `-shutdown.sleep` | `0s` | Hard deadline. Shutdown after specific duration (e.g., `2h`). |
| `-shutdown.at` | *(Disabled)* | Hard deadline. Shutdown at specific time (Format `HH:MM`). |

### Observability
| Flag | Default | Description |
| :--- | :--- | :--- |
| `-logger.level` | `info` | Log verbosity: `debug`, `info`, `warn`, `error`. |

## Architecture

The codebase follows the **Service Object** pattern to separate configuration, wiring, and runtime logic.

```text
cmd/server/
├── server.go       # Application composition root. Manages wiring (NewApp) and the main event loop.
└── shutdown.go     # The Shutdown Monitor. Manages concurrent timers and signal handling.

internal/
├── config/         # Configuration logic. Strongly typed parsing, validation, and architecture checks.
├── api/            # HTTP Layer. Handles Routing, Templates, and SOAP/XML responses.
│   └── templates/  # Embedded XML templates for UPnP services (SCPD, Device Description).
├── media/          # Domain Layer. Filesystem abstraction, buffering logic, and security boundaries.
└── discovery/      # Network Layer. Pure SSDP (Simple Service Discovery Protocol) implementation.
```

### Key Design Decisions

1.  **Safety First:** File access is jailed via `internal/media/resolver.go` using `os.OpenInRoot`. Attempting to access `../../etc/passwd` fails at the file handle creation level, ensuring robust security.
2.  **Concurrency Hygiene:**
    *   The **SSDP Listener** uses a `select` loop checking `ctx.Err()` to prevent CPU spinning during shutdown.
    *   The **Shutdown Monitor** uses a "Stop-and-Drain" pattern for `time.Timer` management to prevent channel race conditions.
3.  **Protocol Compliance:** The API layer (`internal/api`) strictly handles DLNA-specific headers (`EXT`, `transferMode.dlna.org`) and MIME types to ensure compatibility with strict clients (Samsung TV, LG WebOS, Sony as well as player apps on Roku and Amazon Fire TV sticks).
4.  **Path Obfuscation (Security):** The API never exposes physical file paths to the client. An internal Registry maps ephemeral UUIDs to filesystem locations (/stream?id=550e...), preventing path enumeration attacks and decoupling the URL from disk structure.
5.  **I/O Pressure Relief:** To prevent system lockups on slow media (SD cards), the Stream handler acquires a token from a per-volume semaphore before opening files. If the disk is saturated, the server returns 503 Service Unavailable rather than crashing the OS IO scheduler.

## License

MIT