package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

type reviewQueue struct {
	Capacity int             `json:"capacity"`
	Targets  []string        `json:"targets"`
	Queued   int64           `json:"queued"`
	Attempts []reviewAttempt `json:"attempts"`
}

type reviewAttempt struct {
	Target  string `json:"target"`
	Owner   string `json:"owner"`
	Attempt int    `json:"attempt"`
	Start   int64  `json:"start"`
	End     *int64 `json:"end"`
	Status  string `json:"status"`
}

func (a app) schedule(m *manifest, args []string) error {
	if len(args) < 2 || m.Queue != nil || m.RowsSHA256 == "" {
		return errors.New("schedule requires captured run, capacity and pending deep targets, once per run")
	}
	capacity, err := strconv.Atoi(args[0])
	if err != nil || capacity < 1 {
		return errors.New("schedule capacity must be positive")
	}
	seen := map[string]bool{}
	for _, target := range args[1:] {
		if !slices.Contains(m.Targets, target) || seen[target] {
			return fmt.Errorf("%s: unknown or repeated scheduling target", target)
		}
		seen[target] = true
		state, err := checkpointState(m, target)
		if err != nil {
			return err
		}
		if state == "completed" || state == "done" {
			return fmt.Errorf("%s: already completed or checkpointed", target)
		}
		for _, as := range m.Assignments {
			if as.Target == target {
				return fmt.Errorf("%s: already assigned", target)
			}
		}
	}
	targets := slices.Clone(args[1:])
	type weight struct {
		recovery, builtin bool
		size              int64
	}
	weights := map[string]weight{}
	for _, target := range targets {
		if slices.Contains(m.BuiltinDeep, target) {
			w := weights[target]
			w.builtin = true
			weights[target] = w
		}
		for _, source := range m.Sources {
			if source.Target != target || source.Fetch == nil {
				continue
			}
			row := source.Fetch.Row
			if len(row) > 7 && row[7] != "unchanged" {
				w := weights[target]
				w.size += 1000
				if row[7] != "changed" && row[7] != "new" {
					w.recovery = true
				}
				path := filepath.Join(m.Directory, source.Fetch.Evidence.Path)
				delta := strings.TrimSuffix(path, filepath.Ext(path)) + ".delta"
				info, err := os.Stat(delta)
				if err != nil {
					info, err = os.Stat(path)
				}
				if err == nil {
					w.size += info.Size()
				}
				weights[target] = w
			}
		}
	}
	sort.SliceStable(targets, func(i, j int) bool {
		a, b := weights[targets[i]], weights[targets[j]]
		if a.recovery != b.recovery {
			return a.recovery
		}
		if a.builtin != b.builtin {
			return a.builtin
		}
		return a.size > b.size
	})
	m.Queue = &reviewQueue{Capacity: capacity, Targets: targets, Queued: time.Now().Unix(), Attempts: []reviewAttempt{}}
	return nil
}

func refreshQueue(m *manifest) error {
	if m.Queue == nil {
		return errors.New("run has no review schedule")
	}
	for i := range m.Queue.Attempts {
		attempt := &m.Queue.Attempts[i]
		if attempt.Status != "running" && attempt.Status != "completed" {
			continue
		}
		state, err := checkpointState(m, attempt.Target)
		if err != nil {
			return err
		}
		if state != "done" && state != "completed" && attempt.Status == "completed" {
			attempt.Status = "stale"
		}
		if (state == "done" || state == "completed") && attempt.Status == "running" {
			attempt.Status = "completed"
			// A checkpoint records completion; coordinator observation is not its time.
			if info, err := os.Stat(filepath.Join(m.Directory, checkpointMarker(attempt.Target))); err == nil {
				end := info.ModTime().Unix()
				if end >= attempt.Start {
					attempt.End = &end
				}
			}
		}
	}
	return nil
}

func (a app) dispatch(m *manifest, args []string) (string, error) {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		return "", errors.New("dispatch requires a worker name")
	}
	if err := refreshQueue(m); err != nil {
		return "", err
	}
	active := 0
	for _, attempt := range m.Queue.Attempts {
		if attempt.Status == "running" || attempt.Status == "stale" {
			active++
			if attempt.Owner == args[0] {
				return "", fmt.Errorf("%s: worker still active", args[0])
			}
		}
	}
	if active >= m.Queue.Capacity {
		return "", errors.New("review capacity is full")
	}
	for _, target := range m.Queue.Targets {
		state, err := checkpointState(m, target)
		if err != nil {
			return "", err
		}
		if state == "done" || state == "completed" {
			continue
		}
		count := 0
		busy := false
		for _, attempt := range m.Queue.Attempts {
			if attempt.Target == target {
				count++
				busy = busy || attempt.Status == "running" || attempt.Status == "stale"
			}
		}
		if busy {
			continue
		}
		found := false
		for i := range m.Assignments {
			if m.Assignments[i].Target == target {
				if count == 0 {
					return "", fmt.Errorf("%s: assigned outside the review schedule", target)
				}
				m.Assignments[i].Owner = args[0]
				found = true
			}
		}
		if !found {
			m.Assignments = append(m.Assignments, assignment{Target: target, Owner: args[0]})
		}
		attempt := reviewAttempt{Target: target, Owner: args[0], Attempt: count + 1, Start: time.Now().Unix(), Status: "running"}
		m.Queue.Attempts = append(m.Queue.Attempts, attempt)
		b, err := json.Marshal(attempt)
		if err != nil {
			return "", err
		}
		if err = save(m); err != nil {
			return "", err
		}
		return string(b) + "\n", nil
	}
	return "", save(m)
}

func releaseReview(m *manifest, args []string) error {
	if len(args) != 2 || args[1] != "--stopped" {
		return errors.New("release requires target and --stopped after confirming its worker exited")
	}
	if err := refreshQueue(m); err != nil {
		return err
	}
	attempt := latestAttempt(m, args[0])
	if attempt == nil || (attempt.Status != "running" && attempt.Status != "stale") {
		return fmt.Errorf("%s: no unfinished review to release", args[0])
	}
	marker := filepath.Join(m.Directory, checkpointMarker(args[0]))
	if _, err := os.Lstat(marker); err == nil {
		archive := filepath.Join(m.Directory, fmt.Sprintf("%s.attempt-%d.done", args[0], attempt.Attempt))
		if _, err := os.Lstat(archive); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s: checkpoint archive already exists or is unreadable", archive)
		}
		if err := os.Rename(marker, archive); err != nil {
			return fmt.Errorf("%s: archive checkpoint: %w", marker, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s: %w", marker, err)
	}
	end := time.Now().Unix()
	attempt.End = &end
	attempt.Status = "interrupted"
	return nil
}

func latestAttempt(m *manifest, target string) *reviewAttempt {
	if m.Queue == nil {
		return nil
	}
	for i := len(m.Queue.Attempts) - 1; i >= 0; i-- {
		if m.Queue.Attempts[i].Target == target {
			return &m.Queue.Attempts[i]
		}
	}
	return nil
}
