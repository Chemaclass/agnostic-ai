package mailbox

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

func (t Thread) includes(ref conversations.Ref) bool {
	return slices.Contains(t.Between[:], ref)
}

func (t Thread) valid(id string) bool {
	if t.SchemaVersion != schemaVersion || t.ID != id || t.Cap < threadCap || t.Cap%threadCap != 0 ||
		t.Between[0] == t.Between[1] || !supportedRef(t.Between[0]) || !supportedRef(t.Between[1]) || len(t.Messages) == 0 {
		return false
	}
	switch t.Status {
	case Open, Paused, Closing, Closed:
	default:
		return false
	}
	for i, m := range t.Messages {
		if m.N != i+1 || !t.includes(m.From.Ref) || !t.includes(m.To) || m.From.Ref == m.To ||
			m.From.By != "human" && m.From.By != "agent" || m.Model != "" && m.From.By != "agent" || !m.validType() {
			return false
		}
		switch m.State {
		case Held, Waiting, Staged, Rejected, Canceled:
		case Reserved:
			if m.ReservedAt == nil {
				return false
			}
		case Delivered:
			if m.DeliveredAt == nil {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func (m Message) validType() bool {
	switch m.Type {
	case Question, Finding, Done:
		return m.Refs == nil && m.Gives == nil && m.Handoff == ""
	case ReviewRequest:
		return m.Body == "" && m.Refs != nil && m.Gives == nil && m.Handoff == "" &&
			objectID.MatchString(m.Refs.Commit) && objectID.MatchString(m.Refs.Base)
	case Handoff:
		if m.Refs != nil || m.Gives == nil || validateClaimPaths(*m.Gives) != nil {
			return false
		}
		switch m.Handoff {
		case Offered, Declined:
			return m.AcceptedAt == nil && m.AcceptedBy == nil
		case Accepted:
			return m.AcceptedAt != nil && !m.AcceptedAt.IsZero() && m.AcceptedBy != nil && *m.AcceptedBy == m.To
		}
		return false
	default:
		return false
	}
}

func (t *Thread) recordModel(ref conversations.Ref, name, statedBy string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if t.Models == nil {
		t.Models = map[string]Model{}
	}
	key := string(ref.Tool) + ":" + ref.ID
	// The same-model warning rests on the person's label; each agent message still carries its own model.
	if statedBy == "agent" && t.Models[key].StatedBy == "human" {
		return
	}
	t.Models[key] = Model{Name: name, StatedBy: statedBy}
}

func (t Thread) checkSend(from conversations.Ref, req SendRequest, by string) *Refusal {
	if !t.includes(from) || !t.includes(req.To) {
		return NewRefusal("not-in-thread")
	}
	if refused := t.openOnly(); refused != nil {
		return refused
	}
	if req.Type == Done {
		// One agent done that waits for approval is enough; more would only pile up.
		if by == "agent" && slices.ContainsFunc(t.Messages, func(m Message) bool { return m.Type == Done && m.State == Held }) {
			return NewRefusal("closed")
		}
		return nil
	}
	if t.contentCount() >= t.Cap {
		return capRefusal(t.Cap)
	}
	return nil
}

func (t Thread) contentCount() int {
	count := 0
	for _, m := range t.Messages {
		if m.Type != Done && m.State != Rejected {
			count++
		}
	}
	return count
}

func capRefusal(capacity int) *Refusal {
	refused := NewRefusal("cap")
	refused.Reason = fmt.Sprintf("This thread has %d messages, the limit for this thread.", capacity)
	return refused
}

func (t Thread) openOnly() *Refusal {
	switch t.Status {
	case Paused:
		return NewRefusal("paused")
	case Closing, Closed:
		return NewRefusal("closed")
	default:
		return nil
	}
}

func (t *Thread) addEvent(at time.Time, action string, n int) {
	t.addEventBy(at, "human", action, n)
}

func (t *Thread) addEventBy(at time.Time, by, action string, n int) {
	t.Events = append(t.Events, Event{At: at, By: by, Action: action, N: n})
}

func (t *Thread) settle() {
	if t.Status == Closing && !slices.ContainsFunc(t.Messages, func(m Message) bool {
		return m.State == Held || m.State == Waiting || m.State == Reserved
	}) {
		t.Status = Closed
	}
}

func (s Store) Change(ctx context.Context, id, action string) error {
	if refused := checkStrings(id, action); refused != nil {
		return refused
	}
	release, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	thread, err := s.loadThread(id)
	if err != nil {
		return err
	}

	switch action {
	case "pause":
		if thread.Status == Paused {
			return nil
		}
		if refused := thread.openOnly(); refused != nil {
			return refused
		}
		thread.Status = Paused
	case "resume":
		if thread.Status != Paused {
			if refused := thread.openOnly(); refused != nil {
				return refused
			}
			return nil
		}
		thread.Status = Open
	case "stop":
		if thread.Status == Closed {
			return nil
		}
		thread.Status = Closed
		for i := range thread.Messages {
			// A reserved message may be printing now; its hook settles it.
			switch thread.Messages[i].State {
			case Held, Waiting:
				thread.Messages[i].State = Canceled
			}
		}
	case "raise-cap":
		if refused := thread.openOnly(); refused != nil {
			return refused
		}
		if thread.contentCount() < thread.Cap {
			return nil
		}
		if thread.Cap > math.MaxInt-threadCap {
			return capRefusal(thread.Cap)
		}
		thread.Cap += threadCap
	default:
		return &Refusal{"format", "This is not a mailbox thread action.", "Use Pause, Resume, Stop, or Allow 8 more."}
	}

	thread.addEvent(time.Now().UTC(), action, 0)
	return s.writeThread(thread)
}

func (s Store) PrepareStage(ctx context.Context, id string, n int) (Stage, error) {
	if err := ctx.Err(); err != nil {
		return Stage{}, err
	}
	if refused := checkStrings(id); refused != nil {
		return Stage{}, refused
	}
	if !threadID.MatchString(id) {
		return Stage{}, NewRefusal("format")
	}
	release, present, err := s.lockForRead(ctx)
	if err != nil {
		return Stage{}, err
	}
	if !present {
		return Stage{}, missingThread()
	}
	defer release()
	thread, m, err := s.loadMessage(id, n)
	if err != nil {
		return Stage{}, err
	}
	return thread.stageFor(m)
}

// TypeStage holds the mailbox lock through a caller-bounded terminal dispatch.
// A true result means text moved, even if persisting that receipt failed.
func (s Store) TypeStage(ctx context.Context, id string, n int, dispatch func(Stage) bool) (bool, error) {
	if refused := checkStrings(id); refused != nil {
		return false, refused
	}
	release, err := s.lock(ctx)
	if err != nil {
		return false, err
	}
	defer release()

	thread, m, err := s.loadMessage(id, n)
	if err != nil {
		return false, err
	}
	stage, err := thread.stageFor(m)
	if err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if !dispatch(stage) {
		return false, nil
	}
	thread.markStaged(m, "type")
	return true, s.writeThread(thread)
}

func (s Store) ConfirmStage(ctx context.Context, id string, n int, how string) error {
	if refused := checkStrings(id, how); refused != nil {
		return refused
	}
	if how != "copy" && how != "type" {
		return NewRefusal("format")
	}
	release, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	thread, m, err := s.loadMessage(id, n)
	if err != nil {
		return err
	}
	if m.State == Staged {
		return nil
	}
	if refused := thread.canStage(m); refused != nil {
		return refused
	}
	thread.markStaged(m, how)
	return s.writeThread(thread)
}

func (s Store) loadMessage(id string, n int) (Thread, *Message, error) {
	thread, err := s.loadThread(id)
	if err != nil {
		return Thread{}, nil, err
	}
	message, refused := thread.message(n)
	if refused != nil {
		return Thread{}, nil, refused
	}
	return thread, message, nil
}

func (t Thread) stageFor(m *Message) (Stage, error) {
	if refused := t.canStage(m); refused != nil {
		return Stage{}, refused
	}
	text, err := renderBlock(t.ID, *m)
	if err != nil {
		return Stage{}, err
	}
	return Stage{Text: text, To: m.To}, nil
}

func (t *Thread) markStaged(m *Message, how string) {
	m.State = Staged
	m.ReservedAt = nil
	t.settle()
	t.addEvent(time.Now().UTC(), how, m.N)
}

func (t *Thread) message(n int) (*Message, *Refusal) {
	if n < 1 || n > len(t.Messages) {
		return nil, &Refusal{"not-in-thread", "That message is not in this thread.", "Refresh the thread and choose an available message."}
	}
	return &t.Messages[n-1], nil
}

func (t Thread) canStage(m *Message) *Refusal {
	switch t.Status {
	case Paused:
		return NewRefusal("paused")
	case Closed:
		return NewRefusal("closed")
	}
	if !m.deliverable(time.Now().UTC()) {
		return &Refusal{"format", "That message is no longer waiting.", "Refresh the thread and choose a waiting message."}
	}
	return nil
}

func (s Store) Approve(ctx context.Context, id string, n int, action string) error {
	if refused := checkStrings(id, action); refused != nil {
		return refused
	}
	if action != "approve" && action != "reject" {
		return &Refusal{"format", "This is not an approval action.", "Use Approve or Reject."}
	}
	release, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	thread, m, err := s.loadMessage(id, n)
	if err != nil {
		return err
	}
	at := time.Now().UTC()
	if action == "reject" {
		switch m.State {
		case Rejected:
			return nil
		case Held:
		default:
			return &Refusal{"format", "That message is not waiting for approval.", "Refresh the thread to see its current state."}
		}
		m.State = Rejected
		stable := NewRefusal("rejected")
		from := m.From.Ref
		thread.Refusals = append(thread.Refusals, RefusalRecord{At: at, From: &from, Type: m.Type, Guard: stable.Guard, Reason: stable.Reason})
		thread.addEvent(at, "reject", n)
		thread.settle()
		return s.writeThread(thread)
	}

	switch m.State {
	case Held:
	case Rejected:
		return NewRefusal("rejected")
	case Canceled:
		return NewRefusal("closed")
	default:
		return nil
	}
	switch thread.Status {
	case Paused:
		return NewRefusal("paused")
	case Closed:
		return NewRefusal("closed")
	}
	if thread.contentCount() > thread.Cap {
		return capRefusal(thread.Cap)
	}
	if m.Gives != nil {
		refused, err := s.handoffRefusal(m.From.Ref, *m.Gives)
		if err != nil {
			return err
		}
		if refused != nil {
			return refused
		}
	}
	m.State = Waiting
	m.QueuedAt = at
	thread.recordModel(m.From.Ref, m.Model, "agent")
	thread.addEvent(at, "approve", n)
	if m.Type == Done {
		thread.Status = Closing
		thread.addEvent(at, "close", n)
	}
	return s.writeThread(thread)
}

func (s Store) Queue(ctx context.Context, id string, n int) error {
	if refused := checkStrings(id); refused != nil {
		return refused
	}
	release, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	thread, m, err := s.loadMessage(id, n)
	if err != nil {
		return err
	}
	switch m.State {
	case Waiting:
		return nil
	case Staged:
	default:
		return &Refusal{"format", "Only a staged message can be queued for delivery.", "Refresh the thread to see its current state."}
	}
	switch thread.Status {
	case Paused:
		return NewRefusal("paused")
	case Closed:
		return NewRefusal("closed")
	}
	at := time.Now().UTC()
	m.State = Waiting
	m.QueuedAt = at
	thread.addEvent(at, "queue", n)
	return s.writeThread(thread)
}
