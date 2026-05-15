# downloader-qbittorrent

qBittorrent download client connector for MuxCore.

## Env Vars

- `MUXCORE_QBITTORRENT_ADDR` — qBittorrent Web UI URL (default: `http://localhost:8080`)
- `MUXCORE_QBITTORRENT_USER` — username (default: `admin`)
- `MUXCORE_QBITTORRENT_PASS` — password

## Usage

```go
import "github.com/Muxcore-Media/downloader-qbittorrent"

mod := qbittorrent.NewModule()
mgr.Register(mod, nil)
```
