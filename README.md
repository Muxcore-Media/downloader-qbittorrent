# Downloader qBittorrent

MuxCore sidecar that bridges [qBittorrent](https://www.qbittorrent.org/) via the WebUI API.

Implements `muxcore.downloader.v1.DownloaderService` from `contracts-downloader` (AddTorrent, status, pause/resume, delete) plus SettingsProvider.

## Network safety / CI

- **Unit tests and CI never talk to a real qBittorrent.** Use `DOWNLOADER_ENGINE=fixture` / `QBIT_FIXTURE=1` (in-memory) or the httptest mock (`internal/qbit.NewMockServer`).
- Live WebUI is **operator opt-in only**: set `QBIT_URL` or `QBITTORRENT_URL` (+ username/password). Leave URL empty for soft-empty health (optional peer).
- When `QBIT_URL` / `QBITTORRENT_URL` is set, live mode wins even if the host defaults `QBIT_FIXTURE=1`.

## Fixture mode

| Env | Effect |
|-----|--------|
| `DOWNLOADER_ENGINE=fixture` (or `fake`) | In-memory backend; AddTorrent completes instantly |
| `QBIT_FIXTURE=1` | Same as fixture engine (unless a live URL is set) |

When a publisher is injected (tests or mesh adapter), AddTorrent emits:

| Event | When |
|-------|------|
| `download.started` | Torrent accepted |
| `download.completed` | Fixture finishes immediately (unless `paused`); live/mock path polls WebUI until completed |
| `download.failed` | Live/mock path when qBittorrent reports `error` / `missingFiles` |

## Config

| Env | Setting | Default |
|-----|---------|---------|
| `QBIT_URL` / `QBITTORRENT_URL` | `base_url` | — (empty = unconfigured / soft-empty) |
| `QBIT_USERNAME` / `QBITTORRENT_USERNAME` | `username` | `admin` |
| `QBIT_PASSWORD` / `QBITTORRENT_PASSWORD` | `password` | — (operator opt-in) |
| `QBIT_GRPC_ADDR` / `MUXCORE_GRPC_ADDR_OVERRIDE` | gRPC listen | `:9462` |
| `QBIT_HTTP_ADDR` / `MUXCORE_HTTP_ADDR` | health listen | `:9463` |
| `DOWNLOADER_ENGINE` / `QBIT_FIXTURE` | fixture mode | off (unless no live URL) |

## Capabilities

- `downloader`
- `downloader.torrent`
- `downloader.qbittorrent`
- `settings`

## Build / test

```bash
CGO_ENABLED=0 go test ./...   # fixture + httptest only; no live qBit
CGO_ENABLED=0 go build -o bin/downloader-qbittorrent ./cmd/module
```

## Status

v0.1.0 — WebUI client + contracts DownloaderService + fixture path. Optional peer (not default host).
