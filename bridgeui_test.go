// SPDX-License-Identifier: MIT
// Copyright (C) 2026 SukramJ

package bridgeui

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func getJSON(t *testing.T, url string, v any) int {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if v != nil && res.StatusCode == http.StatusOK {
		if err := json.NewDecoder(res.Body).Decode(v); err != nil {
			t.Fatalf("%s: %v", url, err)
		}
	}
	return res.StatusCode
}

// GET /api/ui carries the language and the bridge's layout.
func TestUIEndpoint(t *testing.T) {
	layout := Layout{
		Brand: "mtec2mqtt", Product: "M-TEC Energybutler", Icon: "⚡",
		Sections: []Section{{ID: "live", Title: "section.live", Groups: []string{"now"}}},
	}
	ts := newTestServer(Options{Language: "DE", Layout: layout}, &fakeBackend{})
	defer ts.Close()
	var ui uiView
	getJSON(t, ts.URL+"/api/ui", &ui)
	if ui.Language != "de" || ui.Layout.Brand != "mtec2mqtt" || len(ui.Layout.Sections) != 1 {
		t.Errorf("ui = %+v", ui)
	}

	ts2 := newTestServer(Options{Language: "fr"}, &fakeBackend{})
	defer ts2.Close()
	var ui2 uiView
	getJSON(t, ts2.URL+"/api/ui", &ui2)
	if ui2.Language != "en" || ui2.Layout.Sections == nil {
		t.Errorf("fallback ui = %+v", ui2)
	}
}

// The served bundle is the built-in one merged with the bridge's
// translations, a bridge key overriding a built-in one.
func TestI18nMerge(t *testing.T) {
	ts := newTestServer(Options{Translations: map[string]map[string]string{
		"de": {"group.now": "Aktuell", "nav.system": "Anlage"},
	}}, &fakeBackend{})
	defer ts.Close()

	var de map[string]string
	if code := getJSON(t, ts.URL+"/i18n/de.json", &de); code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if de["group.now"] != "Aktuell" || de["nav.system"] != "Anlage" || de["btn.set"] != "Setzen" {
		t.Errorf("merged de = %v", de)
	}
	var en map[string]string
	getJSON(t, ts.URL+"/i18n/en.json", &en)
	if _, ok := en["group.now"]; ok || en["btn.set"] != "Set" {
		t.Errorf("en carries de's translation or lacks the built-ins: %v", en)
	}
	for _, path := range []string{"/i18n/fr.json", "/i18n/de.txt"} {
		if code := getJSON(t, ts.URL+path, nil); code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", path, code)
		}
	}
}

func TestLoadTranslations(t *testing.T) {
	fsys := fstest.MapFS{
		"i18n/en.json": {Data: []byte(`{"group.now":"Live"}`)},
		"i18n/de.json": {Data: []byte(`{"group.now":"Aktuell"}`)},
	}
	tr, err := LoadTranslations(fsys, "i18n")
	if err != nil {
		t.Fatal(err)
	}
	if tr["en"]["group.now"] != "Live" || tr["de"]["group.now"] != "Aktuell" {
		t.Errorf("translations = %v", tr)
	}
	if tr, err := LoadTranslations(fstest.MapFS{}, "i18n"); err != nil || len(tr) != 0 {
		t.Errorf("empty fs: %v, %v", tr, err)
	}
	bad := fstest.MapFS{"i18n/en.json": {Data: []byte(`{"n": 1}`)}}
	if _, err := LoadTranslations(bad, "i18n"); err == nil {
		t.Error("a non-string bundle was accepted")
	}
}

// startedBackend reports a start time, so the server fills the uptime.
type startedBackend struct{ fakeBackend }

func (startedBackend) Health() Health {
	return Health{Status: "ok", StartedAt: time.Now().Add(-90 * time.Second)}
}

func TestHealthUptimeFilled(t *testing.T) {
	ts := newTestServer(Options{}, &startedBackend{})
	defer ts.Close()
	var h Health
	getJSON(t, ts.URL+"/api/health", &h)
	if h.UptimeSeconds < 89 || h.UptimeSeconds > 120 {
		t.Errorf("uptime = %d", h.UptimeSeconds)
	}
}

// A single-device bridge may write without naming the device.
func TestWriteWithoutDevice(t *testing.T) {
	fb := &fakeBackend{}
	ts := newTestServer(Options{}, fb)
	defer ts.Close()
	res, err := http.Post(ts.URL+"/api/write", "application/json", strings.NewReader(`{"key":"mode","value":true}`))
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK || fb.lastDevice != "" || fb.lastKey != "mode" || fb.lastVal != "1" {
		t.Errorf("status=%d device=%q key=%q val=%q", res.StatusCode, fb.lastDevice, fb.lastKey, fb.lastVal)
	}
}

func TestBasicAuthRealm(t *testing.T) {
	ts := newTestServer(Options{User: "u", Password: "p", Realm: "keba2mqtt"}, &fakeBackend{})
	defer ts.Close()
	res, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if got := res.Header.Get("WWW-Authenticate"); !strings.Contains(got, `realm="keba2mqtt"`) {
		t.Errorf("WWW-Authenticate = %q", got)
	}
}

func TestNotifierCoalescesAndReleases(t *testing.T) {
	var n Notifier
	ch, release := n.Subscribe()
	n.Notify()
	n.Notify()
	select {
	case <-ch:
	default:
		t.Fatal("no signal")
	}
	select {
	case <-ch:
		t.Fatal("two signals were not coalesced")
	default:
	}
	release()
	release()
	n.Notify()
	select {
	case <-ch:
		t.Fatal("a released subscription was signalled")
	default:
	}
}

func TestProblemLog(t *testing.T) {
	added := 0
	l := ProblemLog{Size: 3, OnAdd: func() { added++ }}
	if got := l.List(); got == nil || len(got) != 0 {
		t.Errorf("empty list = %v", got)
	}
	for i := range 5 {
		l.Add(Problem{Event: strconv.Itoa(i)})
	}
	got := l.List()
	if len(got) != 3 || got[0].Event != "4" || got[2].Event != "2" || got[0].At.IsZero() || added != 5 {
		t.Errorf("list = %+v, added = %d", got, added)
	}
	var zero ProblemLog
	for range DefaultProblemLogSize + 1 {
		zero.Add(Problem{})
	}
	if len(zero.List()) != DefaultProblemLogSize {
		t.Errorf("zero-value log kept %d", len(zero.List()))
	}
}

// Both built-in bundles carry the same keys, and every key app.js and
// index.html look up literally is in them — a missing one would show as the
// raw key.
func TestBuiltInBundlesComplete(t *testing.T) {
	load := func(lang string) map[string]string {
		b, err := staticFS.ReadFile("static/i18n/" + lang + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]string
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("%s.json: %v", lang, err)
		}
		return m
	}
	en, de := load("en"), load("de")
	for k := range en {
		if _, ok := de[k]; !ok {
			t.Errorf("de.json lacks %q", k)
		}
	}
	for k := range de {
		if _, ok := en[k]; !ok {
			t.Errorf("en.json lacks %q", k)
		}
	}

	var used []string
	for _, f := range []string{"static/app.js", "static/index.html"} {
		b, err := staticFS.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		re := regexp.MustCompile(`\btf?\("([a-zA-Z]+\.[a-zA-Z]+)"|data-i18n(?:-title)?="([^"]+)"|"((?:val|status)\.[a-z]+)"`)
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			used = append(used, m[1]+m[2]+m[3])
		}
	}
	for _, k := range used {
		if _, ok := en[k]; !ok {
			t.Errorf("%q is used but in no bundle", k)
		}
	}
}

// The SPA's own pages are reachable, the i18n bundles through the merge.
func TestAssetsServed(t *testing.T) {
	ts := newTestServer(Options{}, &fakeBackend{})
	defer ts.Close()
	for _, path := range []string{"/", "/app.js", "/app.css", "/i18n/en.json", "/i18n/de.json"} {
		res, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Errorf("%s: status = %d", path, res.StatusCode)
		}
	}
}

// A bridge's own routes sit behind the server's auth and headers, and a
// mutating one guarded by SameOrigin refuses a foreign origin.
func TestRoutesAndSameOrigin(t *testing.T) {
	var posted int
	ts := newTestServer(Options{User: "u", Password: "p", Routes: func(mux *http.ServeMux) {
		mux.HandleFunc("GET /api/devices", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, `{"devices":[]}`)
		})
		mux.Handle("POST /api/devices/{device}/set", SameOrigin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			posted++
			w.WriteHeader(http.StatusAccepted)
		})))
	}}, &fakeBackend{})
	defer ts.Close()

	do := func(method, path, ctype, origin string, auth bool) (int, http.Header) {
		req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(`{}`))
		if ctype != "" {
			req.Header.Set("Content-Type", ctype)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if auth {
			req.SetBasicAuth("u", "p")
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		_, _ = io.Copy(io.Discard, res.Body)
		return res.StatusCode, res.Header
	}
	if code, _ := do(http.MethodGet, "/api/devices", "", "", false); code != http.StatusUnauthorized {
		t.Errorf("own route without auth: %d", code)
	}
	if code, h := do(http.MethodGet, "/api/devices", "", "", true); code != http.StatusOK || h.Get("Content-Security-Policy") == "" {
		t.Errorf("own route: %d, CSP %q", code, h.Get("Content-Security-Policy"))
	}
	if code, _ := do(http.MethodPost, "/api/devices/d/set", "application/json", "http://evil.example", true); code != http.StatusForbidden {
		t.Errorf("foreign origin: %d", code)
	}
	if code, _ := do(http.MethodPost, "/api/devices/d/set", "text/plain", "", true); code != http.StatusUnsupportedMediaType {
		t.Errorf("text/plain: %d", code)
	}
	if code, _ := do(http.MethodPost, "/api/devices/d/set", "application/json", "", true); code != http.StatusAccepted || posted != 1 {
		t.Errorf("same origin: %d, posted %d", code, posted)
	}
}

// degradedBackend reports a degraded bridge.
type degradedBackend struct{ fakeBackend }

func (degradedBackend) Health() Health { return Health{Status: "degraded"} }

func TestHealthProbe(t *testing.T) {
	for _, c := range []struct {
		probe bool
		b     Backend
		want  int
	}{
		{false, &degradedBackend{}, http.StatusOK},
		{true, &degradedBackend{}, http.StatusServiceUnavailable},
		{true, &fakeBackend{}, http.StatusOK},
	} {
		ts := newTestServer(Options{HealthProbe: c.probe}, c.b)
		var h Health
		if code := getJSON(t, ts.URL+"/api/health", &h); code != c.want {
			t.Errorf("probe=%v: status %d, want %d", c.probe, code, c.want)
		}
		ts.Close()
	}
}

// tableBackend serves a device with tables whose lists are nil.
type tableBackend struct{ fakeBackend }

func (tableBackend) Snapshot() Snapshot {
	return Snapshot{Devices: []Device{{ID: "site", Tables: []Table{
		{ID: "clients", Title: "table.clients"},
		{ID: "aps", Columns: []Column{{Label: "Name"}, {Label: "tx", Format: FormatBitrate, Align: "right"}}, Rows: []Row{{Tone: "muted"}}},
	}}}}
}

// Tables arrive with lists rather than null, so the SPA can iterate them,
// and the layout places them and links the bridge's own pages.
func TestTablesAndLinks(t *testing.T) {
	layout := Layout{
		Sections: []Section{{ID: "network", Title: "section.network", Tables: []string{"aps", "clients"}}},
		Links:    []Link{{Href: "schedules.html", Title: "nav.schedules"}},
	}
	ts := newTestServer(Options{Layout: layout}, &tableBackend{})
	defer ts.Close()

	res, err := http.Get(ts.URL + "/api/snapshot")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	for _, want := range []string{
		`"columns":[]`, `"rows":[]`, `"cells":[]`, `"format":"bitrate"`, `"align":"right"`, `"tone":"muted"`,
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("snapshot lacks %s: %s", want, body)
		}
	}

	var ui uiView
	getJSON(t, ts.URL+"/api/ui", &ui)
	if len(ui.Layout.Links) != 1 || ui.Layout.Links[0].Href != "schedules.html" ||
		len(ui.Layout.Sections[0].Tables) != 2 {
		t.Errorf("ui = %+v", ui)
	}
}

// Every colour in app.css is a token: a literal colour may only appear in a
// custom-property declaration, so the Home Assistant theme app.js copies
// onto the tokens reaches every element.
func TestStylesheetColoursAreTokens(t *testing.T) {
	b, err := staticFS.ReadFile("static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	css := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(string(b), "")
	colour := regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b|\b(?:rgba?|hsla?|oklch|oklab|lab|lch|hwb)\(|\b(?:white|black)\b`)
	for _, decl := range regexp.MustCompile(`[{;]\s*([-a-zA-Z]+)\s*:([^;{}]*)`).FindAllStringSubmatch(css, -1) {
		prop, value := decl[1], decl[2]
		if strings.HasPrefix(prop, "--") {
			continue
		}
		if c := colour.FindString(value); c != "" {
			t.Errorf("%s: %s hard-codes %q; use a token", prop, strings.TrimSpace(value), c)
		}
	}
}

// Each token app.js maps a Home Assistant variable onto is declared in
// app.css, and both palettes define the same set of tokens.
func TestHomeAssistantTokenMapping(t *testing.T) {
	js, err := staticFS.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	css, err := staticFS.ReadFile("static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)const HA_TOKENS = \[(.*?)\n\];`).FindSubmatch(js)
	if block == nil {
		t.Fatal("HA_TOKENS not found in app.js")
	}
	mapped := regexp.MustCompile(`\["(--[a-z-]+)", \[`).FindAllSubmatch(block[1], -1)
	if len(mapped) < 10 {
		t.Fatalf("HA_TOKENS maps %d tokens", len(mapped))
	}
	for _, m := range mapped {
		if !regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(string(m[1])) + `:`).Match(css) {
			t.Errorf("app.js maps %s, which app.css does not declare", m[1])
		}
	}

	palette := func(theme string) []string {
		m := regexp.MustCompile(`(?s)\[data-theme="` + theme + `"\] \{(.*?)\}`).FindSubmatch(css)
		if m == nil {
			t.Fatalf("no %s palette", theme)
		}
		decls := regexp.MustCompile(`(--[a-z-]+):`).FindAllSubmatch(m[1], -1)
		names := make([]string, 0, len(decls))
		for _, d := range decls {
			names = append(names, string(d[1]))
		}
		return names
	}
	if l, d := strings.Join(palette("light"), ","), strings.Join(palette("dark"), ","); l != d {
		t.Errorf("light palette %s differs from dark %s", l, d)
	}
}
