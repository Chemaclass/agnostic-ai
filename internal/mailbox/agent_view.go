package mailbox

import (
	"context"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

// AgentThread is what one participant may read of a thread. Content the
// session has not been shown yet stays out, so a message waiting for
// approval or delivery cannot be read around the hooks.
type AgentThread struct {
	ID       string               `json:"id"`
	Status   string               `json:"status"`
	Cap      int                  `json:"cap"`
	Count    int                  `json:"count"`
	Between  [2]conversations.Ref `json:"between"`
	Models   map[string]Model     `json:"models,omitempty"`
	Messages []Message            `json:"messages"`
}

type AgentThreadList struct {
	Threads []AgentThread `json:"threads"`
	Warning string        `json:"warning,omitempty"`
}

func (t Thread) viewFor(me conversations.Ref) AgentThread {
	view := AgentThread{ID: t.ID, Status: t.Status, Cap: t.Cap, Count: t.contentCount(), Between: t.Between, Models: t.Models}
	view.Messages = make([]Message, len(t.Messages))
	for i, m := range t.Messages {
		if m.From.Ref != me && m.State != Delivered && m.State != Staged {
			m.Body, m.Refs = "", nil
			// Paths a session accepted are its own claims now.
			if m.Handoff != Accepted || m.AcceptedBy == nil || *m.AcceptedBy != me {
				m.Gives = nil
			}
		}
		view.Messages[i] = m
	}
	return view
}

func (s Store) ThreadsFor(ctx context.Context, me conversations.Ref) (AgentThreadList, error) {
	if refused := ValidateRef(me); refused != nil {
		return AgentThreadList{}, refused
	}
	list, err := s.ThreadsOf(ctx, me)
	if err != nil {
		return AgentThreadList{}, err
	}
	view := AgentThreadList{Threads: make([]AgentThread, 0, len(list.Threads)), Warning: list.Warning}
	for _, thread := range list.Threads {
		view.Threads = append(view.Threads, thread.viewFor(me))
	}
	return view, nil
}

func (s Store) ThreadFor(ctx context.Context, id string, me conversations.Ref) (AgentThread, error) {
	thread, err := s.memberThread(ctx, id, me)
	if err != nil {
		return AgentThread{}, err
	}
	return thread.viewFor(me), nil
}

// Peer names the other participant, so a reply needs only the thread.
func (s Store) Peer(ctx context.Context, id string, me conversations.Ref) (conversations.Ref, error) {
	thread, err := s.memberThread(ctx, id, me)
	if err != nil {
		return conversations.Ref{}, err
	}
	if thread.Between[0] == me {
		return thread.Between[1], nil
	}
	return thread.Between[0], nil
}

func (s Store) memberThread(ctx context.Context, id string, me conversations.Ref) (Thread, error) {
	if refused := ValidateRef(me); refused != nil {
		return Thread{}, refused
	}
	if refused := checkStrings(id); refused != nil {
		return Thread{}, refused
	}
	if !threadID.MatchString(id) {
		return Thread{}, NewRefusal("format")
	}
	release, present, err := s.lockForRead(ctx)
	if err != nil {
		return Thread{}, err
	}
	if !present {
		return Thread{}, missingThread()
	}
	defer release()
	thread, err := s.loadThread(id)
	if err != nil {
		return Thread{}, err
	}
	if !thread.includes(me) {
		return Thread{}, NewRefusal("not-in-thread")
	}
	return thread, nil
}
