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
