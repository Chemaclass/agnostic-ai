package mailbox

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

var statsStart = time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)

func near(got, want float64) bool { return math.Abs(got-want) < 1e-9 }

func statsAt(seconds int) time.Time { return statsStart.Add(time.Duration(seconds) * time.Second) }

func statsMessage(n int, kind, state, by string, from, to conversations.Ref, at time.Time) Message {
	m := Message{N: n, Type: kind, From: Actor{Ref: from, By: by}, To: to, State: state, At: at}
	switch kind {
	case Question, Finding, Handoff:
		m.Body = "secret body text " + kind
	}
	if kind == ReviewRequest {
		m.Refs = &ReviewRefs{Worktree: "/Users/me/private/worktree", Commit: strings.Repeat("1", 40), Base: strings.Repeat("2", 40), Paths: []string{"internal/hidden.go"}}
	}
	if kind == Handoff {
		m.Gives = &Gives{Worktree: "/Users/me/private/worktree", Paths: []string{"internal/hidden"}}
		m.Handoff = Offered
	}
	if state != Held {
		m.QueuedAt = at
	}
	switch state {
	case Reserved:
		m.ReservedAt = &at
	case Delivered:
		delivered := at.Add(30 * time.Second)
		m.DeliveredAt = &delivered
	}
	return m
}

func writeStatsThread(t *testing.T, s Store, thread Thread) {
	t.Helper()
	if thread.SchemaVersion == "" {
		thread.SchemaVersion, thread.Cap, thread.Between = schemaVersion, threadCap, [2]conversations.Ref{claude, codex}
	}
	if thread.Status == "" {
		thread.Status = Open
	}
	if err := s.writeThread(thread); err != nil {
		t.Fatal(err)
	}
}

func writeStatsAudit(t *testing.T, s Store, records ...auditRecord) {
	t.Helper()
	if err := writeJSON(filepath.Join(s.Dir, "audit.json"), records); err != nil {
		t.Fatal(err)
	}
}

func readStats(t *testing.T, s Store, since time.Time) Stats {
	t.Helper()
	stats, err := s.Stats(t.Context(), since)
	if err != nil {
		t.Fatal(err)
	}
	return stats
}

func TestStats_AnEmptyOrMissingMailboxIsZeroAndNull(t *testing.T) {
	for name, dir := range map[string]string{"missing": filepath.Join(t.TempDir(), "absent"), "empty": t.TempDir()} {
		t.Run(name, func(t *testing.T) {
			stats := readStats(t, Store{Dir: dir}, time.Time{})
			data, err := json.Marshal(stats)
			if err != nil {
				t.Fatal(err)
			}
			want := `{"schemaVersion":"mailbox-stats/v1","since":null,` +
				`"threads":{"total":0,"open":0,"paused":0,"closing":0,"closed":0,"reachedCapWithoutDone":0},` +
				`"messages":{"total":0,"byType":{"question":0,"finding":0,"review_request":0,"handoff":0,"done":0},` +
				`"byState":{"held":0,"waiting":0,"reserved":0,"staged":0,"delivered":0,"rejected":0,"cancelled":0},` + //nolint:misspell // mailbox/v1 spelling.
				`"bySender":{"human":0,"agent":0}},` +
				`"refusals":{"total":0,"withThread":0,"withoutThread":0,"byGuard":{},"perSendAttempt":null},` +
				`"deliverySeconds":{"count":0,"median":null,"p90":null},` +
				`"humanActions":{"pause":0,"resume":0,"stop":0,"approve":0,"reject":0,"raiseCap":0,"queue":0,"per10Threads":null},` +
				`"handoffs":{"offered":0,"accepted":0,"declined":0,"conflicts":0,"conflictsPer10Handoffs":null},` +
				`"reviews":{"requests":0,"crossModel":0,"sameModel":0,"unknownModel":0,"firstThreadToFirstEmittedReviewSeconds":null}}`
			if string(data) != want {
				t.Errorf("stats =\n%s\nwant\n%s", data, want)
			}
			if _, err := os.Stat(filepath.Join(dir, "threads")); !os.IsNotExist(err) {
				t.Errorf("reading stats created the mailbox: %v", err)
			}
		})
	}
}

// statsFixture holds every state, type and guard across three threads.
func statsFixture(t *testing.T) Store {
	t.Helper()
	s := Store{Dir: t.TempDir()}
	messages := []Message{
		statsMessage(1, Question, Delivered, "human", claude, codex, statsAt(0)),
		statsMessage(2, Finding, Staged, "agent", codex, claude, statsAt(60)),
		statsMessage(3, Question, Held, "agent", claude, codex, statsAt(120)),
		statsMessage(4, Finding, Rejected, "agent", codex, claude, statsAt(180)),
		statsMessage(5, Question, Waiting, "human", claude, codex, statsAt(240)),
		statsMessage(6, Question, Reserved, "human", claude, codex, statsAt(300)),
	}
	messages[0].DeliveredAt = new(statsAt(10))
	writeStatsThread(t, s, Thread{
		ID: "th_20261121_00000001", Messages: messages,
		Refusals: []RefusalRecord{
			{At: statsAt(200), Guard: "secret"},
			{At: statsAt(210), Guard: "cap"},
			{At: statsAt(220), Guard: "rejected"},
		},
		Events: []Event{
			{At: statsAt(130), By: "human", Action: "pause"},
			{At: statsAt(140), By: "human", Action: "resume"},
			{At: statsAt(150), By: "human", Action: "approve", N: 3},
			{At: statsAt(190), By: "human", Action: "reject", N: 4},
			{At: statsAt(195), By: "human", Action: "raise-cap"},
			{At: statsAt(196), By: "human", Action: "queue", N: 2},
			{At: statsAt(197), By: "human", Action: "type", N: 2},
			{At: statsAt(198), By: "agent", Action: "send", N: 2},
		},
	})

	offered := statsMessage(1, Handoff, Delivered, "agent", claude, codex, statsAt(400))
	accepted := statsMessage(2, Handoff, Delivered, "agent", claude, codex, statsAt(410))
	accepted.Handoff, accepted.AcceptedAt, accepted.AcceptedBy = Accepted, new(statsAt(500)), &codex
	declined := statsMessage(3, Handoff, Delivered, "human", claude, codex, statsAt(420))
	declined.Handoff = Declined
	done := statsMessage(4, Done, Canceled, "human", claude, codex, statsAt(430))
	writeStatsThread(t, s, Thread{
		ID: "th_20261121_00000002", Status: Closed, Messages: []Message{offered, accepted, declined, done},
		Events: []Event{
			{At: statsAt(450), By: "agent", Action: "conflict", N: 2},
			{At: statsAt(460), By: "human", Action: "stop"},
		},
	})

	cross := statsMessage(1, ReviewRequest, Delivered, "human", claude, codex, statsAt(600))
	cross.DeliveredAt = new(statsAt(900))
	writeStatsThread(t, s, Thread{
		ID: "th_20261121_00000003", Status: Paused, Messages: []Message{cross},
		Models: map[string]Model{
			"claude:" + claude.ID: {Name: "claude-opus-5-5", StatedBy: "human"},
			"codex:" + codex.ID:   {Name: "gpt-5.5", StatedBy: "human"},
		},
	})
	writeStatsAudit(t, s,
		auditRecord{At: statsAt(50), Guard: "unknown-session", Type: Question},
		auditRecord{At: statsAt(55), Guard: "format"},
	)
	return s
}

func TestStats_CountsEveryStateTypeGuardAndAction(t *testing.T) {
	stats := readStats(t, statsFixture(t), time.Time{})

	if got := stats.Threads; got != (ThreadStats{Total: 3, Open: 1, Paused: 1, Closed: 1}) {
		t.Errorf("threads = %+v", got)
	}
	m := stats.Messages
	if m.Total != 11 || m.ByType != (TypeCounts{Question: 4, Finding: 2, ReviewRequest: 1, Handoff: 3, Done: 1}) ||
		m.ByState != (StateCounts{Held: 1, Waiting: 1, Reserved: 1, Staged: 1, Delivered: 5, Rejected: 1, Canceled: 1}) ||
		m.BySender != (SenderCounts{Human: 6, Agent: 5}) {
		t.Errorf("messages = %+v", m)
	}
	r := stats.Refusals
	if r.Total != 4 || r.WithThread != 2 || r.WithoutThread != 2 || len(r.ByGuard) != 4 ||
		r.ByGuard["secret"] != 1 || r.ByGuard["cap"] != 1 || r.ByGuard["unknown-session"] != 1 || r.ByGuard["format"] != 1 {
		t.Errorf("refusals = %+v", r)
	}
	if r.PerSendAttempt == nil || !near(*r.PerSendAttempt, 4.0/15) {
		t.Errorf("perSendAttempt = %v, want 4/15", r.PerSendAttempt)
	}
	// Delivered: 10s, 30s, 30s, 30s, 300s.
	d := stats.DeliverySeconds
	if d.Count != 5 || d.Median == nil || *d.Median != 30 || d.P90 == nil || *d.P90 != 300 {
		t.Errorf("deliverySeconds = %+v", d)
	}
	h := stats.HumanActions
	if h.Pause != 1 || h.Resume != 1 || h.Stop != 1 || h.Approve != 1 || h.Reject != 1 || h.RaiseCap != 1 || h.Queue != 1 ||
		h.Per10Threads == nil || !near(*h.Per10Threads, 70.0/3) {
		t.Errorf("humanActions = %+v", h)
	}
	ho := stats.Handoffs
	if ho.Offered != 1 || ho.Accepted != 1 || ho.Declined != 1 || ho.Conflicts != 1 || ho.ConflictsPer10Handoffs == nil || !near(*ho.ConflictsPer10Handoffs, 10.0/3) {
		t.Errorf("handoffs = %+v", ho)
	}
	rv := stats.Reviews
	if rv.Requests != 1 || rv.CrossModel != 1 || rv.SameModel != 0 || rv.UnknownModel != 0 ||
		rv.FirstThreadToFirstEmittedReviewSeconds == nil || *rv.FirstThreadToFirstEmittedReviewSeconds != 900 {
		t.Errorf("reviews = %+v", rv)
	}
	if stats.AuditFull || len(stats.Unreadable) != 0 {
		t.Errorf("notes = full %t, unreadable %v", stats.AuditFull, stats.Unreadable)
	}
}

func TestStats_HoldsNoMessageTextPathsOrSessionIDs(t *testing.T) {
	data, err := json.Marshal(readStats(t, statsFixture(t), time.Time{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"secret body text", "/Users/me", "internal/hidden", claude.ID, codex.ID, "claude-opus", "gpt-5.5"} {
		if strings.Contains(string(data), leak) {
			t.Errorf("stats hold %q: %s", leak, data)
		}
	}
}

func TestStats_DeliveryStartsWhenAHeldMessageIsQueued(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	m := statsMessage(1, Question, Delivered, "agent", claude, codex, statsAt(0))
	m.QueuedAt = statsAt(3600)
	m.DeliveredAt = new(statsAt(3605))
	writeStatsThread(t, s, Thread{ID: "th_20261121_00000001", Messages: []Message{m}})
	if d := readStats(t, s, time.Time{}).DeliverySeconds; d.Median == nil || *d.Median != 5 {
		t.Errorf("deliverySeconds = %+v, want 5 from queuedAt", d)
	}
}

func TestStats_OnlyHeldMessagesHaveNoDeliveryTimes(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	writeStatsThread(t, s, Thread{ID: "th_20261121_00000001", Messages: []Message{
		statsMessage(1, Question, Held, "agent", claude, codex, statsAt(0)),
		statsMessage(2, Question, Held, "agent", claude, codex, statsAt(1)),
	}})
	stats := readStats(t, s, time.Time{})
	if stats.Messages.ByState.Held != 2 || stats.DeliverySeconds.Count != 0 || stats.DeliverySeconds.Median != nil ||
		stats.Reviews.FirstThreadToFirstEmittedReviewSeconds != nil {
		t.Errorf("held-only stats = %+v", stats)
	}
}

func TestPercentiles_OneTwoAndManyValues(t *testing.T) {
	for _, test := range []struct {
		values      []float64
		median, p90 float64
	}{
		{[]float64{7}, 7, 7},
		{[]float64{10, 4}, 7, 10},
		{[]float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 5.5, 9},
		{[]float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}, 6, 10},
	} {
		d := durationStats(test.values)
		if d.Count != len(test.values) || *d.Median != test.median || *d.P90 != test.p90 {
			t.Errorf("%v: %+v, want median %v p90 %v", test.values, d, test.median, test.p90)
		}
	}
}

func TestStats_ReviewModels(t *testing.T) {
	for _, test := range []struct {
		name     string
		models   map[string]Model
		stated   string
		want     ReviewStats
		crossAny bool
	}{
		{"the same model across two tools", map[string]Model{
			"claude:" + claude.ID: {Name: "GPT-5.5 ", StatedBy: "human"}, "codex:" + codex.ID: {Name: "gpt-5.5", StatedBy: "human"},
		}, "", ReviewStats{Requests: 1, SameModel: 1}, false},
		{"a missing model on the reader's side", map[string]Model{
			"claude:" + claude.ID: {Name: "claude-opus-5-5", StatedBy: "human"},
		}, "", ReviewStats{Requests: 1, UnknownModel: 1}, false},
		{"no model at all", nil, "", ReviewStats{Requests: 1, UnknownModel: 1}, false},
		{"the message's own stated model wins", map[string]Model{
			"claude:" + claude.ID: {Name: "gpt-5.5", StatedBy: "agent"}, "codex:" + codex.ID: {Name: "gpt-5.5", StatedBy: "agent"},
		}, "claude-opus-5-5", ReviewStats{Requests: 1, CrossModel: 1}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := Store{Dir: t.TempDir()}
			review := statsMessage(1, ReviewRequest, Delivered, "agent", claude, codex, statsAt(0))
			review.Model = test.stated
			writeStatsThread(t, s, Thread{ID: "th_20261121_00000001", Messages: []Message{review}, Models: test.models})
			got := readStats(t, s, time.Time{}).Reviews
			if (got.FirstThreadToFirstEmittedReviewSeconds != nil) != test.crossAny {
				t.Errorf("first cross-model review = %v", got.FirstThreadToFirstEmittedReviewSeconds)
			}
			got.FirstThreadToFirstEmittedReviewSeconds = nil
			if got != test.want {
				t.Errorf("reviews = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestStats_SinceLimitsEveryNumber(t *testing.T) {
	stats := readStats(t, statsFixture(t), statsAt(400))
	if stats.Since == nil || *stats.Since != "2026-11-21T10:06:40Z" {
		t.Errorf("since = %v", stats.Since)
	}
	if stats.Threads.Total != 2 || stats.Messages.Total != 5 || stats.Messages.ByType.Question != 0 ||
		stats.Refusals.Total != 0 || stats.Refusals.PerSendAttempt == nil || *stats.Refusals.PerSendAttempt != 0 ||
		stats.HumanActions.Stop != 1 || stats.HumanActions.Pause != 0 || stats.Handoffs.Conflicts != 1 ||
		stats.DeliverySeconds.Count != 4 || stats.Reviews.Requests != 1 {
		t.Errorf("stats since = %+v", stats)
	}
	if got := stats.Reviews.FirstThreadToFirstEmittedReviewSeconds; got == nil || *got != 500 {
		t.Errorf("first cross-model review since = %v, want 500", got)
	}
}

func TestStats_SkipsAndNamesACorruptThread(t *testing.T) {
	s := statsFixture(t)
	if err := os.WriteFile(s.threadPath("th_20261121_0000000f"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	stats := readStats(t, s, time.Time{})
	if stats.Threads.Total != 3 || len(stats.Unreadable) != 1 || stats.Unreadable[0] != "th_20261121_0000000f.json" {
		t.Errorf("threads %d, unreadable %v", stats.Threads.Total, stats.Unreadable)
	}
}

func TestStats_SaysWhenTheAuditFileIsFull(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	records := make([]auditRecord, MaxAudit)
	for i := range records {
		records[i] = auditRecord{At: statsAt(i), Guard: "format"}
	}
	writeStatsAudit(t, s, records...)
	stats := readStats(t, s, time.Time{})
	if !stats.AuditFull || stats.Refusals.WithoutThread != MaxAudit || stats.Refusals.ByGuard["format"] != MaxAudit {
		t.Errorf("full audit = %t %+v", stats.AuditFull, stats.Refusals)
	}
}

func TestAnswerHandoff_AConflictLeavesAnEventWithoutPaths(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	root := claimRepo(t)
	offer := claimAndOffer(t, s, root, "scope/a.go")
	file := Gives{root, []string{"scope/a.go"}}
	if err := s.Release(t.Context(), claude, file); err != nil {
		t.Fatal(err)
	}
	if err := s.Claim(t.Context(), mailThird, file, root, "human"); err != nil {
		t.Fatal(err)
	}
	requireGuard(t, s.AnswerHandoff(t.Context(), offer.Thread, offer.N, "accept"), "ownership")
	requireGuard(t, s.AnswerHandoff(t.Context(), offer.Thread, offer.N, "accept"), "ownership")
	thread := onlyThread(t, s)
	if got := countEvents(thread, "conflict"); got != 1 {
		t.Errorf("a retried accept left %d conflict events, want 1", got)
	}
	last := thread.Events[len(thread.Events)-1]
	if last.Action != "conflict" || last.By != "human" || last.N != offer.N || last.At.IsZero() {
		t.Errorf("conflict event = %+v", last)
	}
	if got := readStats(t, s, time.Time{}).Handoffs.Conflicts; got != 1 {
		t.Errorf("conflicts = %d", got)
	}
}

func countEvents(thread Thread, action string) int {
	count := 0
	for _, e := range thread.Events {
		if e.Action == action {
			count++
		}
	}
	return count
}

func TestStats_ARejectedDoneDoesNotEndACappedThread(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	messages := []Message{}
	for n := 1; n <= threadCap; n++ {
		messages = append(messages, statsMessage(n, Question, Delivered, "agent", claude, codex, statsAt(n)))
	}
	messages = append(messages, statsMessage(threadCap+1, Done, Rejected, "agent", claude, codex, statsAt(threadCap+1)))
	writeStatsThread(t, s, Thread{ID: "th_20261121_00000001", Messages: messages})
	if got := readStats(t, s, time.Time{}).Threads.ReachedCapWithoutDone; got != 1 {
		t.Errorf("reachedCapWithoutDone = %d, want 1", got)
	}
}

func TestStats_AnUnreadableAuditFileCountsNothing(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if err := os.WriteFile(filepath.Join(s.Dir, "audit.json"), []byte(`[{"at":"2026-11-21T10:00:00Z","guard":"format","extra":1}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	stats := readStats(t, s, time.Time{})
	if stats.Refusals.WithoutThread != 0 || len(stats.Unreadable) != 1 {
		t.Errorf("refusals %+v, unreadable %v", stats.Refusals, stats.Unreadable)
	}
}

func TestStats_ReadsWithoutTakingTheLockOrFinishingATransfer(t *testing.T) {
	s := statsFixture(t)
	if err := writeJSON(filepath.Join(s.Dir, "claims.json"), claimsFile{SchemaVersion: claimsVersion, Claims: []Claim{}, Pending: &pendingTransfer{Thread: "th_20261121_0000000f", N: 1}}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(s.Dir, "claims.json"))
	if stats := readStats(t, s, time.Time{}); stats.Threads.Total != 3 {
		t.Errorf("threads = %d", stats.Threads.Total)
	}
	after, _ := os.ReadFile(filepath.Join(s.Dir, "claims.json"))
	if string(before) != string(after) {
		t.Error("stats changed claims.json")
	}
	if _, err := os.Stat(filepath.Join(s.Dir, ".lock")); !os.IsNotExist(err) {
		t.Errorf("stats created the lock file: %v", err)
	}
}
