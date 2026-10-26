# Changelog

All notable changes to this project are documented in this file. The
format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [0.3.0] - 2026-10-26

### Added

- Tables: `Device.Tables` carries lists of like things — a site's access
  points, its clients — with typed columns (`Column.Format`: `duration`,
  `age`, `bitrate`, `i18n`; `Align: "right"`) and toned rows.
  `Section.Tables` places them; unclaimed tables go under System. From ten
  rows on a table offers a filter, which keeps its focus across live
  updates.
- `Layout.Links`: navigation entries to pages a bridge serves itself
  through `Options.Routes` (a schedule editor, a login).

### Changed

- A bridge without any writable item is read-only: the Control section and
  its navigation entry are hidden.

## [0.2.0] - 2026-10-26

### Added

- `Options.Routes`: a bridge registers its own routes on the server's
  mux, behind the same Basic auth, hardening headers and write deadline —
  for a documented API of its own beside the dashboard's.
- `SameOrigin`: the JSON + same-origin guard of `POST /api/write`,
  exported for a bridge's own mutating routes.
- `Options.HealthProbe`: `GET /api/health` answers 503 unless the status
  is `ok`, so it serves as a readiness probe.

## [0.1.0] - 2026-10-26

### Added

- First release: the embedded dashboard of the go-*2mqtt bridges, extracted
  from go-mtec2mqtt and go-zendure2mqtt 0.13.0 so every bridge serves the
  same UI.
- `Backend`, the contract a bridge implements as an adapter: devices with
  their values (`Snapshot` → `Device` → `Entry`), writable `Item`s,
  `Health` and configuration `Tile`s, one `Write` path classed by
  `ErrUnknownDevice` / `ErrNotWritable` / `ErrInvalidValue` /
  `ErrNotReady`, and `Changes` for the live stream.
- `Layout`: brand, icon and the bridge's data sections between the fixed
  Status, Control and System sections; per-sub-unit cards (battery packs)
  and sections hidden while empty.
- `Server`: `/api/health|status|snapshot|items|config|ui`, the SSE stream
  `/api/events`, `POST /api/write`, merged i18n bundles, HTTP Basic auth,
  the same-origin/JSON guard on writes, hardening headers, write deadlines
  and content-hash ETags on the assets.
- The single-page app: sidebar navigation, light/dark theme, German and
  English, a device picker for bridges with several devices, the problem
  log, control rows for numbers, selects, switches and buttons, and a
  confirmation for items marked `Confirm`.
- `Notifier`, `ProblemLog` and `LoadTranslations` helpers.
