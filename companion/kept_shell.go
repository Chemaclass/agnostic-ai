package main

import (
	"fmt"
	"net/http"
	"slices"
	"time"
)

// keptShell is a stuck shell the person chose to keep running: the list
// stops asking about it until WakeAt. The page picks WakeAt and Keeps;
// the companion only stores them, so they survive a restart.
type keptShell struct {
	// Tool and ID name the chat that holds the shell, so a full list can
	// tell when the shell is gone from it.
	Tool string `json:"tool"`
	ID   string `json:"id"`
	// PID and ProcStart name the shell's process. A reused PID has
	// another start time, so it never matches an old record.
	PID       int       `json:"pid"`
	ProcStart string    `json:"procStart"`
	WakeAt    time.Time `json:"wakeAt"`
	Keeps     int       `json:"keeps"`
}

func (k keptShell) is(pid int, procStart string) bool {
	return k.PID == pid && k.ProcStart == procStart
}

// keptUntil reports whether a record holds the shell quiet at now.
func keptUntil(kept []keptShell, pid int, procStart string, now time.Time) bool {
	return slices.ContainsFunc(kept, func(k keptShell) bool { return k.is(pid, procStart) && now.Before(k.WakeAt) })
}

// keptRecordAge is how long a record stays after its wake time. A full
// list drops the record of a shell gone from a live chat; this drops
// the record of a chat that ended.
const keptRecordAge = 24 * time.Hour

// longestKeep is the longest wait the page asks for; keepSkew allows
// for the page's clock reading a little apart from the companion's.
const (
	longestKeep = 4 * time.Hour
	keepSkew    = time.Minute
)

// maxKeptShells bounds the file; past it the records that wake first go.
const maxKeptShells = 64

// shortestKeptWake keeps a timer that fires early, after a clock change,
// from firing again at once.
const shortestKeptWake = time.Second

// nextKeptWake is how long until the first record after now wakes.
func nextKeptWake(kept []keptShell, now time.Time) (time.Duration, bool) {
	var next time.Duration
	found := false
	for _, k := range kept {
		if wait := k.WakeAt.Sub(now); wait > 0 && (!found || wait < next) {
			next, found = wait, true
		}
	}
	return next, found
}

type shellRef struct {
	PID       int    `json:"pid"`
	ProcStart string `json:"procStart"`
}

func (s *shell) keptShells() []keptShell {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.settings.KeptShells)
}

// changeKeptShells saves a change to the kept shells and tells the pages
// the list in effect: the new one, or the old one when the save failed.
// It tells them under the lock, so two changes reach them in order.
func (s *shell) changeKeptShells(change func([]keptShell) []keptShell) ([]keptShell, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	before := s.settings.KeptShells
	now := time.Now()
	s.settings.KeptShells = slices.DeleteFunc(change(slices.Clone(before)), func(k keptShell) bool { return now.Sub(k.WakeAt) > keptRecordAge })
	kept := slices.Clone(s.settings.KeptShells)
	base := s.settings
	base.KeptShells = before
	err := saveSettingsChange(s.settingsPath, base, func(saved *settings) { saved.KeptShells = kept })
	if err != nil {
		s.settings.KeptShells = before
	}
	kept = slices.Clone(s.settings.KeptShells)
	s.armKeptWakeLocked(now)
	if s.tellKeptShells != nil {
		s.tellKeptShells(kept)
	}
	if err != nil {
		return kept, fmt.Errorf("save the kept shells: %w", err)
	}
	return kept, nil
}

// armKeptWake reads the list again at the next wake time, so a due
// shell can bring a hidden circle back while no new list comes.
func (s *shell) armKeptWake() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.armKeptWakeLocked(time.Now())
}

func (s *shell) armKeptWakeLocked(now time.Time) {
	if s.keptWake != nil {
		s.keptWake.Stop()
	}
	wait, ok := nextKeptWake(s.settings.KeptShells, now)
	if !ok || s.inbox == nil {
		return
	}
	s.keptWake = time.AfterFunc(max(wait, shortestKeptWake), func() {
		s.listRead(s.inbox.held())
		s.armKeptWake()
	})
}

func writeKeptShells(w http.ResponseWriter, kept []keptShell) {
	if kept == nil {
		kept = []keptShell{}
	}
	writeJSON(w, http.StatusOK, struct {
		Shells []keptShell `json:"shells"`
	}{kept})
}

func (s *shell) getKeptShells(w http.ResponseWriter, _ *http.Request) {
	writeKeptShells(w, s.keptShells())
}

// keepShell adds a record, or replaces the one for the same shell.
func (s *shell) keepShell(w http.ResponseWriter, r *http.Request) {
	const shape = "{tool, id, pid, procStart, wakeAt, keeps}"
	body, ok := readBody[keptShell](w, r, shape)
	if !ok {
		return
	}
	if body.Tool == "" || body.ID == "" || body.PID <= 0 || body.ProcStart == "" || body.WakeAt.IsZero() || body.Keeps < 1 {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "send " + shape})
		return
	}
	if now := time.Now(); !body.WakeAt.After(now) || body.WakeAt.After(now.Add(longestKeep+keepSkew)) {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "wakeAt must be within the next 4 hours"})
		return
	}
	kept, err := s.changeKeptShells(func(kept []keptShell) []keptShell {
		kept = slices.DeleteFunc(kept, func(k keptShell) bool { return k.is(body.PID, body.ProcStart) })
		if len(kept) >= maxKeptShells {
			slices.SortFunc(kept, func(a, b keptShell) int { return a.WakeAt.Compare(b.WakeAt) })
			kept = kept[len(kept)-maxKeptShells+1:]
		}
		return append(kept, body)
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: err.Error()})
		return
	}
	writeKeptShells(w, kept)
}

// forgetShells removes the records of the shells named.
func (s *shell) forgetShells(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody[struct {
		Shells []shellRef `json:"shells"`
	}](w, r, "{shells: [{pid, procStart}]}")
	if !ok {
		return
	}
	kept, err := s.changeKeptShells(func(kept []keptShell) []keptShell {
		return slices.DeleteFunc(kept, func(k keptShell) bool {
			return slices.ContainsFunc(body.Shells, func(ref shellRef) bool { return k.is(ref.PID, ref.ProcStart) })
		})
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: err.Error()})
		return
	}
	writeKeptShells(w, kept)
}
