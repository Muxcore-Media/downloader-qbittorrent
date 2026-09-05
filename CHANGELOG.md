# Changelog

## [v0.1.0] — 2026-08-20

### Added
- qBittorrent WebUI API client (login, add, info, pause/resume/delete)
- `contracts-downloader` `DownloaderService` gRPC adapter
- Fixture mode via `DOWNLOADER_ENGINE=fixture` or `QBIT_FIXTURE=1` (no live WebUI)
- httptest mock for offline client/module tests
- Soft-empty health when `QBITTORRENT_URL` unset
- SettingsProvider (`base_url`, `username`, `password`)
- Health endpoint `:9463`
