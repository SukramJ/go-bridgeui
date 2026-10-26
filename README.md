# go-bridgeui

[![ci](https://github.com/SukramJ/go-bridgeui/actions/workflows/ci.yml/badge.svg)](https://github.com/SukramJ/go-bridgeui/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](./LICENSE)

The embedded web dashboard of the `go-*2mqtt` bridges —
[go-mtec2mqtt](https://github.com/SukramJ/go-mtec2mqtt),
[go-keba2mqtt](https://github.com/SukramJ/go-keba2mqtt) and
[go-zendure2mqtt](https://github.com/SukramJ/go-zendure2mqtt). It is
maintained once and embedded by each bridge.

- One HTTP server: a REST API, a Server-Sent-Events stream, HTTP Basic
  auth, a same-origin guard on the write route, hardening headers, write
  deadlines, and revalidated assets.
- One single-page app: status & health, recent problems, live values,
  sub-unit cards (battery packs), configuration, and a control list for
  numbers, selects, switches and buttons, with confirmation where a bridge
  asks for it. Light/dark theme, German and English. A device picker
  appears when a bridge serves several devices.
- Plain HTML/CSS/JS embedded with `go:embed`: no build step, no CDN, no
  third-party Go dependency. It works behind Home Assistant Ingress.

## Use

```go
import "github.com/SukramJ/go-bridgeui"

tr, _ := bridgeui.LoadTranslations(i18nFS, "i18n") // the bridge's own labels

srv := bridgeui.New(bridgeui.Options{
	Bind:     cfg.WebBind,
	User:     cfg.WebUser,
	Password: cfg.WebPassword,
	Realm:    "mtec2mqtt",
	Language: cfg.Language,
	Layout: bridgeui.Layout{
		Brand: "mtec2mqtt", Product: "M-TEC Energybutler", Icon: "⚡",
		Sections: []bridgeui.Section{
			{ID: "live", Title: "section.live", Groups: []string{"now_base", "now_grid"}},
			{ID: "energy", Title: "section.energy", Groups: []string{"day", "total"}},
			{ID: "evcc", Title: "section.evcc", Groups: []string{"evcc"}, Optional: true},
		},
		SystemGroups: []string{"config", "static"},
	},
	Translations: tr,
}, adapter) // adapter implements bridgeui.Backend

go srv.Run(ctx)
```

`Backend` is the whole contract:

| Method | Returns |
|---|---|
| `Snapshot()` | devices → entries (`group`, `sub`, `key`, `name`, `unit`, `value`, `tone`) |
| `Health()` | status (`ok`/`degraded`/`starting`), version, start time, tiles, problems |
| `Config()` | the sanitised configuration as tiles |
| `Items()` | writable items: number/select/switch/button, bounds, options, `confirm` |
| `Write(ctx, device, key, value)` | the bridge's ordinary command path, errors classed by `ErrUnknownDevice` / `ErrNotWritable` / `ErrInvalidValue` / `ErrNotReady` |
| `Changes()` | a coalesced change signal (`Notifier.Subscribe`) |

Labels on tiles and section titles are i18n keys. The built-in bundles
(`static/i18n`) cover the page chrome. A bridge adds its own group, tile and
section labels through `Options.Translations`.

## API

| Route | |
|---|---|
| `GET /api/health` | health |
| `GET /api/status` | `{health, snapshot}` |
| `GET /api/snapshot` | snapshot |
| `GET /api/items` | writable items |
| `GET /api/config` | configuration tiles |
| `GET /api/ui` | `{language, layout}` |
| `GET /api/events` | SSE, `update` events carrying `{health, snapshot}` |
| `POST /api/write` | `{"device", "key", "value"}`, `application/json`, same-origin only |
| `GET /i18n/<lang>.json` | built-in + bridge bundle |

## Development

```sh
make setup   # pinned tooling + git hooks
make check   # vet + gofumpt + golangci-lint + app.js syntax + tests (-race)
```

MIT licensed. See [AI_POLICY.md](AI_POLICY.md) for how AI tools are used here.
