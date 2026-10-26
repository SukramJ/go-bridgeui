// SPDX-License-Identifier: MIT
// Copyright (C) 2026 SukramJ

package bridgeui

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"net/http"
	"path"
	"strings"
)

// languages are the UI languages the built-in bundles cover.
var languages = []string{"en", "de"}

// language maps a configured LANGUAGE to a bundle: "de" or English.
func language(lang string) string {
	if strings.EqualFold(lang, "de") {
		return "de"
	}
	return "en"
}

// buildBundles merges the built-in bundles with a bridge's translations,
// once, so the handler serves bytes. A bridge key overrides a built-in
// one of the same name.
func buildBundles(extra map[string]map[string]string) map[string][]byte {
	out := make(map[string][]byte, len(languages))
	for _, lang := range languages {
		merged := map[string]string{}
		if b, err := staticFS.ReadFile("static/i18n/" + lang + ".json"); err == nil {
			// The built-in bundles are embedded and covered by a test, so
			// a decode error cannot happen in a released build.
			_ = json.Unmarshal(b, &merged)
		}
		maps.Copy(merged, extra[lang])
		b, err := json.Marshal(merged)
		if err != nil {
			continue
		}
		out[lang] = b
	}
	return out
}

// handleI18n serves the merged bundle of one language, /i18n/<lang>.json.
func (s *Server) handleI18n(w http.ResponseWriter, r *http.Request) {
	lang, ok := strings.CutSuffix(r.PathValue("file"), ".json")
	b, known := s.bundles[lang]
	if !ok || !known {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(b)
}

// LoadTranslations reads a bridge's translation bundles, <dir>/<lang>.json
// for every UI language, from fsys — typically an embed.FS. A missing
// language is skipped; a bundle that is not a flat string object is an
// error.
func LoadTranslations(fsys fs.FS, dir string) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	for _, lang := range languages {
		b, err := fs.ReadFile(fsys, path.Join(dir, lang+".json"))
		if err != nil {
			continue
		}
		m := map[string]string{}
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, fmt.Errorf("bridgeui: %s/%s.json: %w", dir, lang, err)
		}
		out[lang] = m
	}
	return out, nil
}
