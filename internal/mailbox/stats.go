package mailbox

import (
	"context"
	"errors"
	"io/fs"
	"math"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

const statsSchemaVersion = "mailbox-stats/v1"

// Stats are counts and durations from the store. They hold no message
// text, paths or session IDs. Durations are seconds.
type Stats struct {
	SchemaVersion   string           `json:"schemaVersion"`
	Since           *string          `json:"since"`
	Threads         ThreadStats      `json:"threads"`
	Messages        MessageStats     `json:"messages"`
	Refusals        RefusalStats     `json:"refusals"`
	DeliverySeconds DurationStats    `json:"deliverySeconds"`
	HumanActions    HumanActionStats `json:"humanActions"`
	Handoffs        HandoffStats     `json:"handoffs"`
	Reviews         ReviewStats      `json:"reviews"`
	// AuditFull says audit.json holds its last MaxAudit refusals, so older ones are gone.
	AuditFull bool `json:"-"`
	// Unreadable names the thread files that were skipped.
	Unreadable []string `json:"-"`
}

type ThreadStats struct {
	Total                 int `json:"total"`
	Open                  int `json:"open"`
	Paused                int `json:"paused"`
	Closing               int `json:"closing"`
	Closed                int `json:"closed"`
	ReachedCapWithoutDone int `json:"reachedCapWithoutDone"`
}

type MessageStats struct {
	Total    int          `json:"total"`
	ByType   TypeCounts   `json:"byType"`
	ByState  StateCounts  `json:"byState"`
	BySender SenderCounts `json:"bySender"`
}

type TypeCounts struct {
	Question      int `json:"question"`
	Finding       int `json:"finding"`
	ReviewRequest int `json:"review_request"`
	Handoff       int `json:"handoff"`
	Done          int `json:"done"`
}

type StateCounts struct {
	Held      int `json:"held"`
	Waiting   int `json:"waiting"`
	Reserved  int `json:"reserved"`
	Staged    int `json:"staged"`
	Delivered int `json:"delivered"`
	Rejected  int `json:"rejected"`
	Canceled  int `json:"cancelled"` //nolint:misspell // mailbox/v1 spelling.
}

type SenderCounts struct {
	Human int `json:"human"`
	Agent int `json:"agent"`
}

type RefusalStats struct {
	Total          int            `json:"total"`
	WithThread     int            `json:"withThread"`
	WithoutThread  int            `json:"withoutThread"`
	ByGuard        map[string]int `json:"byGuard"`
	PerSendAttempt *float64       `json:"perSendAttempt"`
}

type DurationStats struct {
	Count  int      `json:"count"`
	Median *float64 `json:"median"`
	P90    *float64 `json:"p90"`
}

type HumanActionStats struct {
	Pause        int      `json:"pause"`
	Resume       int      `json:"resume"`
	Stop         int      `json:"stop"`
	Approve      int      `json:"approve"`
	Reject       int      `json:"reject"`
	RaiseCap     int      `json:"raiseCap"`
	Queue        int      `json:"queue"`
	Per10Threads *float64 `json:"per10Threads"`
}

type HandoffStats struct {
	Offered                int      `json:"offered"`
	Accepted               int      `json:"accepted"`
	Declined               int      `json:"declined"`
	Conflicts              int      `json:"conflicts"`
	ConflictsPer10Handoffs *float64 `json:"conflictsPer10Handoffs"`
}

type ReviewStats struct {
	Requests                               int      `json:"requests"`
	CrossModel                             int      `json:"crossModel"`
	SameModel                              int      `json:"sameModel"`
	UnknownModel                           int      `json:"unknownModel"`
	FirstThreadToFirstEmittedReviewSeconds *float64 `json:"firstThreadToFirstEmittedReviewSeconds"`
}

// Stats counts what happened at or after since; a zero since counts everything.
func (s Store) Stats(ctx context.Context, since time.Time) (Stats, error) {
	stats := Stats{SchemaVersion: statsSchemaVersion, Refusals: RefusalStats{ByGuard: map[string]int{}}}
	if !since.IsZero() {
		at := since.UTC().Format(time.RFC3339)
		stats.Since = &at
	}
	// Writes replace files atomically and readJSON lets them replace an
	// open file, so counting goes on without the lock and never finishes an
	// interrupted transfer.
	if err := ctx.Err(); err != nil {
		return Stats{}, err
	}
	if err := checkDir(s.Dir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			stats.finish(nil, time.Time{}, nil)
			return stats, nil
		}
		return Stats{}, err
	}

	entries, err := s.threadEntries()
	if err != nil {
		return Stats{}, err
	}
	var delivery []float64
	var first, firstCross time.Time
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return Stats{}, err
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !strings.HasSuffix(entry.Name(), ".json") || !threadID.MatchString(id) {
			continue
		}
		thread, err := s.readThread(id)
		if err != nil {
			stats.Unreadable = append(stats.Unreadable, entry.Name())
			continue
		}
		stats.addThread(thread, since, &delivery, &first, &firstCross)
	}

	var audit []auditRecord
	if err := readJSON(filepath.Join(s.Dir, "audit.json"), &audit); err != nil && !errors.Is(err, fs.ErrNotExist) {
		stats.Unreadable = append(stats.Unreadable, "audit.json")
		audit = nil
	}
	stats.AuditFull = len(audit) >= MaxAudit
	for _, record := range audit {
		if !record.At.Before(since) {
			stats.Refusals.WithoutThread++
			stats.Refusals.ByGuard[record.Guard]++
		}
	}
	stats.finish(delivery, first, &firstCross)
	return stats, nil
}

func (stats *Stats) addThread(thread Thread, since time.Time, delivery *[]float64, first, firstCross *time.Time) {
	counted := false
	for _, m := range thread.Messages {
		if m.At.Before(since) {
			continue
		}
		counted = true
		if first.IsZero() || m.At.Before(*first) {
			*first = m.At
		}
		stats.addMessage(thread, m, delivery, firstCross)
	}
	if counted {
		stats.Threads.Total++
		switch thread.Status {
		case Open:
			stats.Threads.Open++
		case Paused:
			stats.Threads.Paused++
		case Closing:
			stats.Threads.Closing++
		case Closed:
			stats.Threads.Closed++
		}
		if thread.contentCount() >= thread.Cap && !slices.ContainsFunc(thread.Messages, func(m Message) bool { return m.Type == Done && m.State != Rejected }) {
			stats.Threads.ReachedCapWithoutDone++
		}
	}
	for _, refusal := range thread.Refusals {
		// A rejected message was accepted first; Reject is a human action, not a refused send.
		if refusal.At.Before(since) || refusal.Guard == "rejected" {
			continue
		}
		stats.Refusals.WithThread++
		stats.Refusals.ByGuard[refusal.Guard]++
	}
	for _, event := range thread.Events {
		if event.At.Before(since) {
			continue
		}
		if event.Action == "conflict" {
			stats.Handoffs.Conflicts++
		}
		if event.By != "human" {
			continue
		}
		h := &stats.HumanActions
		switch event.Action {
		case "pause":
			h.Pause++
		case "resume":
			h.Resume++
		case "stop":
			h.Stop++
		case "approve":
			h.Approve++
		case "reject":
			h.Reject++
		case "raise-cap":
			h.RaiseCap++
		case "queue":
			h.Queue++
		}
	}
}

func (stats *Stats) addMessage(thread Thread, m Message, delivery *[]float64, firstCross *time.Time) {
	stats.Messages.Total++
	types := &stats.Messages.ByType
	switch m.Type {
	case Question:
		types.Question++
	case Finding:
		types.Finding++
	case ReviewRequest:
		types.ReviewRequest++
	case Handoff:
		types.Handoff++
	case Done:
		types.Done++
	}
	states := &stats.Messages.ByState
	switch m.State {
	case Held:
		states.Held++
	case Waiting:
		states.Waiting++
	case Reserved:
		states.Reserved++
	case Staged:
		states.Staged++
	case Delivered:
		states.Delivered++
		if m.DeliveredAt != nil && !m.QueuedAt.IsZero() {
			*delivery = append(*delivery, m.DeliveredAt.Sub(m.QueuedAt).Seconds())
		}
	case Rejected:
		states.Rejected++
	case Canceled:
		states.Canceled++
	}
	if m.From.By == "agent" {
		stats.Messages.BySender.Agent++
	} else {
		stats.Messages.BySender.Human++
	}
	if m.State == Rejected {
		return
	}
	switch m.Type {
	case Handoff:
		switch m.Handoff {
		case Offered:
			stats.Handoffs.Offered++
		case Accepted:
			stats.Handoffs.Accepted++
		case Declined:
			stats.Handoffs.Declined++
		}
	case ReviewRequest:
		stats.Reviews.Requests++
		from := strings.TrimSpace(m.Model)
		if from == "" {
			from = thread.modelOf(m.From.Ref)
		}
		to := thread.modelOf(m.To)
		switch {
		case from == "" || to == "":
			stats.Reviews.UnknownModel++
		case strings.EqualFold(from, to):
			stats.Reviews.SameModel++
		default:
			stats.Reviews.CrossModel++
			if m.State == Delivered && m.DeliveredAt != nil && (firstCross.IsZero() || m.DeliveredAt.Before(*firstCross)) {
				*firstCross = *m.DeliveredAt
			}
		}
	}
}

func (t Thread) modelOf(ref conversations.Ref) string {
	return strings.TrimSpace(t.Models[string(ref.Tool)+":"+ref.ID].Name)
}

func (stats *Stats) finish(delivery []float64, first time.Time, firstCross *time.Time) {
	stats.DeliverySeconds = durationStats(delivery)
	refusals := &stats.Refusals
	refusals.Total = refusals.WithThread + refusals.WithoutThread
	refusals.PerSendAttempt = ratio(refusals.Total, stats.Messages.Total+refusals.Total, 1)
	h := &stats.HumanActions
	h.Per10Threads = ratio(h.Pause+h.Resume+h.Stop+h.Approve+h.Reject+h.RaiseCap+h.Queue, stats.Threads.Total, 10)
	handoffs := &stats.Handoffs
	handoffs.ConflictsPer10Handoffs = ratio(handoffs.Conflicts, handoffs.Offered+handoffs.Accepted+handoffs.Declined, 10)
	if firstCross != nil && !firstCross.IsZero() && !first.IsZero() {
		seconds := firstCross.Sub(first).Seconds()
		stats.Reviews.FirstThreadToFirstEmittedReviewSeconds = &seconds
	}
}

func ratio(count, over int, scale float64) *float64 {
	if over == 0 {
		return nil
	}
	value := float64(count) / float64(over) * scale
	return &value
}

// durationStats takes the median, averaging the two middle values, and
// the nearest-rank 90th percentile.
func durationStats(values []float64) DurationStats {
	stats := DurationStats{Count: len(values)}
	if len(values) == 0 {
		return stats
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	n := len(sorted)
	median := sorted[n/2]
	if n%2 == 0 {
		median = (sorted[n/2-1] + sorted[n/2]) / 2
	}
	p90 := sorted[int(math.Ceil(0.9*float64(n)))-1]
	stats.Median, stats.P90 = &median, &p90
	return stats
}
