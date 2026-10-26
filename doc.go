// SPDX-License-Identifier: MIT
// Copyright (C) 2026 SukramJ

// Package bridgeui is the embedded web dashboard of the go-*2mqtt bridges:
// one HTTP server and one single-page app, maintained once and embedded by
// every bridge.
//
// A bridge implements [Backend] as an adapter over its own state — devices
// with their values, the writable items, health and configuration tiles,
// one write path — describes its page with a [Layout], and adds its own
// labels as translations. The server does the rest: a small REST API, a
// Server-Sent-Events stream, HTTP Basic auth, a same-origin guard on the
// one mutating route, hardening headers, write deadlines and revalidated
// assets. The app is hand-written vanilla HTML/CSS/JS, embedded into the
// binary — no build step, no third-party dependency — and works behind
// Home Assistant Ingress because every URL it uses is relative.
//
// Routes:
//
//	GET  /api/health    Health
//	GET  /api/status    {health, snapshot}
//	GET  /api/snapshot  Snapshot
//	GET  /api/items     []Item
//	GET  /api/config    []Tile
//	GET  /api/ui        {language, layout}
//	GET  /api/events    SSE, "update" events carrying {health, snapshot}
//	POST /api/write     {device, key, value}, JSON and same-origin only
//	GET  /i18n/<lang>.json, /, /app.js, /app.css
package bridgeui

// Version is the module's release, the fallback asset validator.
const Version = "0.1.0"
