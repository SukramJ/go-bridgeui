# CLAUDE.md — AI Assistant Guide for go-bridgeui

## Project Overview

**go-bridgeui** is the embedded web dashboard of the `go-*2mqtt` bridges
(go-mtec2mqtt, go-keba2mqtt, go-zendure2mqtt): one HTTP server and one
vanilla single-page app, maintained once and embedded by every bridge. It
is a **library module**: no `main` package, no binaries, no Docker image.

A bridge implements `bridgeui.Backend` as an adapter over its own state,
describes its page with a `bridgeui.Layout`, adds its own labels as
translations, and calls `bridgeui.New(opts, backend).Run(ctx)`. The UI
started as go-mtec2mqtt's, was extended for several devices in
go-zendure2mqtt 0.13.0, and was extracted here.

## Key Characteristics

- **Language**: Go 1.27+ (see `go.mod`).
- **Module path**: `github.com/SukramJ/go-bridgeui`, package `bridgeui`.
- **License: MIT.** Every Go source file starts with:
  ```go
  // SPDX-License-Identifier: MIT
  // Copyright (C) 2026 SukramJ
  ```
- **Zero third-party dependencies**, frontend included: hand-written
  HTML/CSS/JS under `static/`, embedded with `go:embed`, no build step, no
  CDN. Keep it that way.
- **Home Assistant Ingress**: every URL the SPA uses is relative, and the
  page is meant to be framed (no `X-Frame-Options`).

## The contract

- `model.go` is the API every bridge depends on: `Backend`, `Snapshot` →
  `Device` → `Entry`, `Item`/`Option`, `Health`/`Tile`/`Problem`,
  `Layout`/`Section`, the `Err*` write classes. JSON field names are what
  `static/app.js` reads — change both together.
- Layout: the status, control and system sections are fixed; a bridge's
  data sections sit between status and control. A group no section claims
  appears under System. `Section.Subs` makes one card per `Entry.Sub`
  (battery packs); `Section.Optional` hides an empty section.
- i18n: `static/i18n/{en,de}.json` are the built-in strings; a bridge's
  `Options.Translations` are merged over them and served at
  `/i18n/<lang>.json`. Tiles and section titles carry i18n keys.
  `TestBuiltInBundlesComplete` checks every literal key app.js/index.html
  use is in the built-in bundles.
- Writes: `POST /api/write {device,key,value}` → `Backend.Write`, which
  must take the bridge's ordinary command path (the same as an MQTT `set`)
  and class its errors with `ErrUnknownDevice`/`ErrNotWritable`/
  `ErrInvalidValue`/`ErrNotReady`. Everything else is a 502 without detail.
- Hardening (do not weaken): JSON + same-origin guard on the write route,
  security headers, Basic auth with constant-time compare, per-request and
  per-SSE-frame write deadlines, `BaseContext` tied to Run's context,
  content-hash ETag + `no-cache` on assets.
- Helpers: `Notifier` (coalesced change signals, `Backend.Changes`),
  `ProblemLog` (bounded recent problems), `LoadTranslations` (embedded
  JSON bundles).

## Repository Structure

```
doc.go           package doc, Version
model.go         the contract (Backend, Snapshot, Item, Health, Layout, errors)
server.go        Options, Server, routes, middleware (auth, same-origin, headers, deadlines), ETag, Run
api.go           JSON handlers, write mapping, SSE stream
i18n.go          bundle merge, /i18n handler, LoadTranslations
notify.go        Notifier, ProblemLog
static/          index.html, app.js, app.css, i18n/{en,de}.json
*_test.go        handler, hardening, SSE, i18n and helper tests
```

## Development Commands

```sh
make setup       # pinned gofumpt/golangci-lint/govulncheck + goimports, git hooks
make test        # go test -race -count=1 ./...
make check       # vet + fmt-check + lint + jscheck + test — the pre-commit/pre-push gate
make cover-check # per-package coverage gate (COVER_MIN, default 85)
make vuln        # govulncheck
```

## Code Conventions

- **Formatting**: `gofumpt` + `goimports -local github.com/SukramJ/go-bridgeui`.
- **Logging**: `log/slog` only.
- **Commit style**: Conventional Commits with a scope (`feat(ui): …`,
  `fix(server): …`, `chore(deps): …`).
- **Releases**: bump `Version` in `doc.go`, add a `## [x.y.z] - date`
  section to `CHANGELOG.md`, then tag `vx.y.z`; `release-on-tag.yml`
  publishes that section. Bridges pin the exact version.
- **API stability**: v0.x. A breaking change to the contract means
  updating go-mtec2mqtt, go-keba2mqtt and go-zendure2mqtt in step.
- **Git hooks**: `make hooks` blocks direct commits on `main`/`master`.
