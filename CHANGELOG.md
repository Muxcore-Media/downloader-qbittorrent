# Changelog

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
