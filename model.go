// SPDX-License-Identifier: MIT
// Copyright (C) 2026 SukramJ

package bridgeui

import (
	"context"
	"errors"
	"time"
)

// Backend is what a bridge hands the server: read access to its live state
// and one write path. A bridge implements it as an adapter over its own
// state, so the bridge's packages never import this module's types beyond
// that adapter.
type Backend interface {
	// Snapshot returns the latest values of every device.
	Snapshot() Snapshot
	// Health returns the operational status surface.
	Health() Health
	// Config returns the sanitised configuration as display tiles: no
	// passwords, no tokens.
	Config() []Tile
	// Items returns what the UI can write.
	Items() []Item
	// Write sets one item of one device, synchronously. device is a
	// [Device.ID] (a single-device bridge may ignore it), key an
	// [Item.Key], value the plain payload an MQTT `set` would carry. Errors
	// are classed by [ErrUnknownDevice], [ErrNotWritable], [ErrInvalidValue]
	// and [ErrNotReady]; anything else is reported as a failed write
	// without its detail.
	Write(ctx context.Context, device, key, value string) error
	// Changes returns a channel that receives a value whenever the state
	// changes, and a func that releases it. [Notifier] implements the
	// coalescing this needs.
	Changes() (events <-chan struct{}, cancel func())
}

// The write errors a [Backend] classes its failures by; the server maps
// them to HTTP status codes.
var (
	// ErrUnknownDevice: no device with that ID (404).
	ErrUnknownDevice = errors.New("unknown device")
	// ErrNotWritable: the key names nothing writable (400).
	ErrNotWritable = errors.New("not writable")
	// ErrInvalidValue: the value does not fit the item (400).
	ErrInvalidValue = errors.New("invalid value")
	// ErrNotReady: the bridge or the device cannot take the write yet
	// (503).
	ErrNotReady = errors.New("not ready")
)

// Snapshot is a point-in-time copy of every device's values.
type Snapshot struct {
	Devices []Device `json:"devices"`
}

// Device is one bridged device. A single-device bridge has exactly one.
type Device struct {
	// ID identifies the device in writes and in the UI's picker — a
	// serial number, typically.
	ID string `json:"id"`
	// Title heads the page while the device is selected ("M-TEC
	// Energybutler GEN3"); Subtitle goes beneath it ("SN … · FW …").
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	// Online is the device's reachability, nil when the bridge does not
	// track it.
	Online *bool `json:"online,omitempty"`
	// UpdatedAt is the device's last report; zero means it has not
	// reported.
	UpdatedAt time.Time `json:"updated_at"`
	// GroupUpdatedAt overrides UpdatedAt per group, for a bridge that
	// polls its groups at different rates.
	GroupUpdatedAt map[string]time.Time `json:"group_updated_at,omitempty"`
	Entries        []Entry              `json:"entries"`
}

// Entry is one value of a device.
type Entry struct {
	// Group places the entry on the page through [Section.Groups].
	Group string `json:"group"`
	// Sub splits a group into one card per sub-unit (a battery pack's
	// serial); only sections with [Section.Subs] show it.
	Sub string `json:"sub,omitempty"`
	// Key identifies the value, and is what an [Item] with the same key
	// reads its current value from.
	Key  string `json:"key"`
	Name string `json:"name,omitempty"`
	Unit string `json:"unit,omitempty"`
	// Value is a number, string or bool; a select item's value is its
	// [Option.Value].
	Value any `json:"value"`
	// Tone colours the value: "ok", "warn", "err" or empty.
	Tone string `json:"tone,omitempty"`
}

// Health is the operational status surface.
type Health struct {
	// Status is "ok", "degraded" or "starting".
	Status  string `json:"status"`
	Version string `json:"version"`
	// StartedAt drives the uptime tile; the server fills UptimeSeconds.
	StartedAt     time.Time `json:"started_at"`
	UptimeSeconds int64     `json:"uptime_seconds"`
	// Tiles are the bridge's own status tiles (connections, topic, …),
	// shown after the status tile and before the uptime tile.
	Tiles    []Tile    `json:"tiles"`
	Problems []Problem `json:"problems"`
}

// Tile is one labelled value of the status or configuration grid.
type Tile struct {
	// Label is an i18n key, or the text itself when the bundles have no
	// such key.
	Label string `json:"label"`
	// Value is shown formatted (numbers localised, booleans as yes/no)
	// unless Text is set.
	Value any `json:"value,omitempty"`
	// Text is an i18n key (or text) shown instead of Value — "val.connected".
	Text string `json:"text,omitempty"`
	// Sub follows the value, after a dot: an address, a topic.
	Sub string `json:"sub,omitempty"`
	// Tone colours the value: "ok", "warn", "err" or empty.
	Tone string `json:"tone,omitempty"`
}

// Problem is one entry of the recent-problem log.
type Problem struct {
	At     time.Time `json:"at"`
	Device string    `json:"device,omitempty"`
	Event  string    `json:"event"`
	Detail string    `json:"detail,omitempty"`
}

// Item is one writable value.
type Item struct {
	// Key is what [Backend.Write] takes and which [Entry.Key] holds its
	// current value.
	Key string `json:"key"`
	// Device limits the item to one device; empty means every device.
	Device string `json:"device,omitempty"`
	Name   string `json:"name"`
	Unit   string `json:"unit,omitempty"`
	// Component is "number", "select", "switch" or "button".
	Component string   `json:"component"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
	Step      *float64 `json:"step,omitempty"`
	Options   []Option `json:"options,omitempty"`
	// Payload is what a button writes ("PRESS").
	Payload string `json:"payload,omitempty"`
	// Confirm makes the UI ask before writing: the item changes how the
	// device operates or what it persists.
	Confirm bool `json:"confirm,omitempty"`
}

// Option is one select choice: Value is what a write takes, Label its
// display.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Layout is how a bridge arranges its page. The status, control and system
// sections are fixed; Sections are the data sections between status and
// control, in order.
type Layout struct {
	// Brand and Product head the sidebar ("mtec2mqtt", "M-TEC
	// Energybutler"); Icon is an emoji for the sidebar and the favicon.
	Brand   string `json:"brand"`
	Product string `json:"product"`
	Icon    string `json:"icon"`
	// Sections are the data sections. A group no section claims is shown
	// under "System".
	Sections []Section `json:"sections"`
	// SystemGroups orders the groups shown under "System"; unclaimed
	// groups follow them.
	SystemGroups []string `json:"system_groups,omitempty"`
}

// Section is one data section of the page.
type Section struct {
	// ID is the section's anchor; Title an i18n key (or text) used for the
	// heading and the navigation entry.
	ID    string `json:"id"`
	Title string `json:"title"`
	// Nav is a shorter i18n key (or text) for the navigation entry; empty
	// means Title.
	Nav string `json:"nav,omitempty"`
	// Groups are the entry groups shown here, in order, one card each.
	Groups []string `json:"groups,omitempty"`
	// Subs makes the section show one card per [Entry.Sub] of every
	// entry that has one, titled by the i18n key SubTitle with {sub}.
	Subs     bool   `json:"subs,omitempty"`
	SubTitle string `json:"sub_title,omitempty"`
	// Optional hides the section, navigation entry included, while the
	// selected device has nothing for it.
	Optional bool `json:"optional,omitempty"`
}
