package mailbox

import (
	"cmp"
	"context"
	"errors"
	"io/fs"
	"slices"
	"strings"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

const (
	deliveryBudget   = 6000
	reservationAge   = 60 * time.Second
	DeliveryLockWait = 500 * time.Millisecond
)

// Delivery is what one hook run reserved. The reservation time is the token
// that keeps a late confirm off a reservation a later run took over.
type Delivery struct {
	Block string
	items []reservation
}

type reservation struct {
	thread string
	n      int
	at     time.Time
}

type candidate struct {
	thread *Thread
	m      *Message
}

// Reserve does not create a missing mailbox.
func (s Store) Reserve(ctx context.Context, to conversations.Ref) (Delivery, error) {
	if err := checkDir(s.Dir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Delivery{}, nil
		}
		return Delivery{}, err
	}
	release, err := s.lockWithin(ctx, DeliveryLockWait)
	if err != nil {
		return Delivery{}, err
	}
	defer release()

	now := time.Now().UTC()
	entries, err := s.threadEntries()
	if err != nil {
		return Delivery{}, err
	}
	var threads []*Thread
	var candidates []candidate
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return Delivery{}, err
		}
		id, ok := strings.CutSuffix(entry.Name(), ".json")
		if !ok || !threadID.MatchString(id) {
			continue
		}
		thread, err := s.readThread(id)
		if err != nil || !thread.includes(to) || thread.Status != Open && thread.Status != Closing {
			continue
		}
		threads = append(threads, &thread)
		for i := range thread.Messages {
			if m := &thread.Messages[i]; m.To == to && m.deliverable(now) {
				candidates = append(candidates, candidate{&thread, m})
			}
		}
	}
	slices.SortFunc(candidates, func(a, b candidate) int {
		return cmp.Or(a.m.QueuedAt.Compare(b.m.QueuedAt), strings.Compare(a.thread.ID, b.thread.ID), cmp.Compare(a.m.N, b.m.N))
	})

	changed := map[string]bool{}
	var delivery Delivery
	var lines []string
	size := len(blockOpening) + len(blockClosing)
	for _, c := range candidates {
		line, err := renderLine(c.thread.ID, *c.m)
		if err != nil || len(line) > maxLineBytes {
			c.m.State = Canceled
			c.m.ReservedAt = nil
			c.thread.addEventBy(now, "hook", "cancel-oversize", c.m.N)
			c.thread.settle()
			changed[c.thread.ID] = true
			continue
		}
		if size+len(line)+1 > deliveryBudget {
			break
		}
		size += len(line) + 1
		c.m.State = Reserved
		c.m.ReservedAt = &now
		changed[c.thread.ID] = true
		lines = append(lines, string(line))
		delivery.items = append(delivery.items, reservation{c.thread.ID, c.m.N, now})
	}
	for _, thread := range threads {
		if changed[thread.ID] {
			if err := s.writeThread(*thread); err != nil {
				return Delivery{}, err
			}
		}
	}
	if len(lines) > 0 {
		delivery.Block = blockOpening + strings.Join(lines, "\n") + blockClosing
	}
	return delivery, nil
}

func (m Message) deliverable(now time.Time) bool {
	return m.State == Waiting || m.State == Reserved && now.Sub(*m.ReservedAt) > reservationAge
}

func (s Store) Confirm(ctx context.Context, d Delivery) error {
	return s.settleDelivery(ctx, d, func(t *Thread, m *Message, at time.Time) {
		m.State = Delivered
		m.DeliveredAt = &at
		t.settle()
	})
}

func (s Store) PutBack(ctx context.Context, d Delivery) error {
	return s.settleDelivery(ctx, d, func(_ *Thread, m *Message, _ time.Time) {
		m.State = Waiting
	})
}

func (s Store) settleDelivery(ctx context.Context, d Delivery, change func(*Thread, *Message, time.Time)) error {
	if len(d.items) == 0 {
		return nil
	}
	release, err := s.lockWithin(ctx, DeliveryLockWait)
	if err != nil {
		return err
	}
	defer release()
	now := time.Now().UTC()
	var errs []error
	for id, items := range groupByThread(d.items) {
		thread, err := s.loadThread(id)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		changed := false
		for _, item := range items {
			m, refused := thread.message(item.n)
			if refused != nil || m.State != Reserved || !m.ReservedAt.Equal(item.at) {
				continue
			}
			m.ReservedAt = nil
			change(&thread, m, now)
			changed = true
		}
		if changed {
			errs = append(errs, s.writeThread(thread))
		}
	}
	return errors.Join(errs...)
}

func groupByThread(items []reservation) map[string][]reservation {
	grouped := map[string][]reservation{}
	for _, item := range items {
		grouped[item.thread] = append(grouped[item.thread], item)
	}
	return grouped
}
