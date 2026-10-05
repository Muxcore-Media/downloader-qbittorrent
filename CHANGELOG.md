# Changelog

## [0.1.5] - 2026-10-05


### Security
- WebUI base URL (`QBIT_URL` / `base_url` setting) is validated with netguard (Integration profile, LAN and loopback allowed): non-http(s) schemes and cloud-metadata/link-local/unspecified targets are rejected; the default HTTP client enforces the same at dial time and on redirects (NFR-SEC-009 / RULE-VAL-2; sdk/go/module v0.6.6).
- AddTorrent `torrent_url` (non-magnet) is validated with the netguard UserURL profile, including DNS resolution of the host, before being handed to qBittorrent; multi-line values (qBittorrent treats newlines as multiple URLs) are refused. New opt-in `DOWNLOADER_INDEXER_HOSTS` allows listed LAN indexer proxies (e.g. Prowlarr).
- AddTorrent `save_path` is confined (pathguard) to `QBIT_DOWNLOAD_ROOTS` (fallback `DOWNLOAD_DIR`): traversal, symlink and sibling-prefix escapes are rejected, and a non-empty save_path is refused when no roots are configured (fail closed). Empty save_path keeps qBittorrent's default (NFR-SEC-008 / RULE-VAL-1).

## [0.1.4] - 2026-10-05


### Security
- gRPC server and peer dials use mesh TLS (meshtls, sdk/go/module v0.6.5) unless the dev insecure flag is set (ADR-0016/0017).

## [0.1.3] - 2026-10-05

### Changed
- Built on core v0.6.14 / sdk/go/module v0.6.4: unregisters on shutdown and re-registers after core restarts (ADR-0022).

## [0.1.2] - 2026-10-05


### Changed
- Reported version comes from muxcore.json (ADR-0021); built on core v0.6.12 / sdk/go/module v0.6.3 (mesh enrollment, ADR-0017).

## [0.1.0] - 2026-10-05

### Changed
- CI runs on GitHub-hosted runners from the umbrella template; retired-origin workflows removed.
- Dependencies resolve from published GitHub tags (no filesystem `replace`); requires core v0.6.0.

## [v0.1.0] — 2026-08-20

### Added
- qBittorrent WebUI API client (login, add, info, pause/resume/delete)
- `contracts-downloader` `DownloaderService` gRPC adapter
- Fixture mode via `DOWNLOADER_ENGINE=fixture` or `QBIT_FIXTURE=1` (no live WebUI)
- httptest mock for offline client/module tests
- Soft-empty health when `QBITTORRENT_URL` unset
- SettingsProvider (`base_url`, `username`, `password`)
- Health endpoint `:9463`
