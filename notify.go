// SPDX-License-Identifier: MIT
// Copyright (C) 2026 SukramJ

package bridgeui

import (
	"sync"
	"time"
)

// Notifier fans a change signal out to subscribers, coalesced: a slow
// subscriber sees one pending signal, never a backlog, and a signal never
// blocks the caller. The zero value is ready to use.
type Notifier struct {
	mu   sync.Mutex
	subs map[chan struct{}]struct{}
}

// Subscribe returns a channel that receives a value after every [Notifier.Notify],
// and a func that releases it; calling the func twice is harmless. Its
// signature is [Backend.Changes]'s, so a backend can return it directly.
func (n *Notifier) Subscribe() (events <-chan struct{}, cancel func()) {
	ch := make(chan struct{}, 1)
	n.mu.Lock()
	if n.subs == nil {
		n.subs = map[chan struct{}]struct{}{}
	}
	n.subs[ch] = struct{}{}
	n.mu.Unlock()
	var once sync.Once
	return ch, func() {
		once.Do(func() {
			n.mu.Lock()
			delete(n.subs, ch)
			n.mu.Unlock()
		})
	}
}

// Notify signals every subscriber without blocking.
func (n *Notifier) Notify() {
	n.mu.Lock()
	defer n.mu.Unlock()
	for ch := range n.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// DefaultProblemLogSize bounds a [ProblemLog] whose Size is zero.
const DefaultProblemLogSize = 20

// ProblemLog keeps the most recent problems, bounded. The zero value is
// ready to use and keeps [DefaultProblemLogSize] entries.
type ProblemLog struct {
	// Size bounds the log; zero means [DefaultProblemLogSize].
	Size int
	// OnAdd, when set, runs after every Add — a [Notifier.Notify], so
	// the UI shows the problem at once.
	OnAdd func()

	mu    sync.Mutex
	items []Problem
}

// Add appends p, stamping it with the current time when At is zero, and
// drops the oldest entries beyond the bound.
func (l *ProblemLog) Add(p Problem) {
	if p.At.IsZero() {
		p.At = time.Now()
	}
	size := l.Size
	if size <= 0 {
		size = DefaultProblemLogSize
	}
	l.mu.Lock()
	l.items = append(l.items, p)
	if n := len(l.items) - size; n > 0 {
		l.items = append(l.items[:0:0], l.items[n:]...)
	}
	l.mu.Unlock()
	if l.OnAdd != nil {
		l.OnAdd()
	}
}

// List returns the problems newest first, never nil.
func (l *ProblemLog) List() []Problem {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Problem, len(l.items))
	for i, p := range l.items {
		out[len(l.items)-1-i] = p
	}
	return out
}
