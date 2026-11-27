package conversations

import (
	"context"
	"errors"
	"runtime"
)

// RenameInTool writes a name into the tool that keeps a conversation.
type RenameInTool func(ctx context.Context, c Conversation, title string) error

// ErrConversationGone says the list no longer holds the conversation.
var ErrConversationGone = errors.New("the conversation is not listed any more")

// ToolRenameError says the tool did not take the name; Err says why.
type ToolRenameError struct{ Err error }

func (e *ToolRenameError) Error() string { return e.Err.Error() }

func (e *ToolRenameError) Unwrap() error { return e.Err }

// ChatRename is one rename of a conversation.
type ChatRename struct {
	Tool Tool
	ID   string
	// Title is a name CleanTitle kept.
	Title  string
	Opts   ScanOptions
	InTool RenameInTool
	// Check may refuse the conversation found; nil takes it.
	Check func(Conversation) error
	// TabLauncher, when set, lets the saved chat name also reach its tab.
	TabLauncher Launcher
	tab         *tabChat
}

// RenameChat renames one conversation as the person's rename does: it
// holds the conversation's rename lock, finds it, writes the name into
// its tool and only then saves it in the marks file. Holding the lock
// across both writes keeps two renames from ending with one name in
// the tool and the other in the list.
func (s *Scanner) RenameChat(ctx context.Context, r ChatRename) (Conversation, error) {
	release, err := s.LockRename(r.Tool, r.ID)
	if err != nil {
		return Conversation{}, err
	}
	defer func() {
		if release != nil {
			release()
		}
	}()
	c, err := s.renameChat(ctx, r)
	if err != nil {
		return c, err
	}
	if host, ok := tabOf(c); ok && r.TabLauncher != nil {
		tabCtx := context.WithoutCancel(ctx)
		if !canReadTabNames(host.App, runtime.GOOS) {
			go func() { _ = TitleTab(tabCtx, c, s.Processes, r.TabLauncher, r.Title) }()
		} else {
			// A tab read must not copy the old name back while this write is pending.
			go func(done func()) {
				defer done()
				if err := TitleTab(tabCtx, c, s.Processes, r.TabLauncher, r.Title); err != nil {
					s.resetTabNameObservation(c, host)
				}
			}(release)
			release = nil
		}
	}
	return c, nil
}

// renameChat runs with the conversation rename lock held.
func (s *Scanner) renameChat(ctx context.Context, r ChatRename) (Conversation, error) {
	c, err := s.findChatToRename(ctx, r)
	if err != nil {
		return c, err
	}
	if r.Check != nil {
		if err := r.Check(c); err != nil {
			return c, err
		}
	}
	if err := r.InTool(ctx, c, r.Title); err != nil {
		return c, &ToolRenameError{Err: err}
	}
	if err := s.Rename(r.Tool, r.ID, r.Title); err != nil {
		return c, err
	}
	return c, nil
}

func (s *Scanner) findChatToRename(ctx context.Context, r ChatRename) (Conversation, error) {
	if r.tab == nil {
		c, ok := s.Find(ctx, r.Tool, r.ID, r.Opts)
		if !ok {
			return Conversation{}, ErrConversationGone
		}
		return c, nil
	}
	s.lock()
	defer s.unlock()
	results, _ := s.readAll(ctx, r.Opts)
	if ctx.Err() == nil {
		for _, chat := range uniqueTabChats(results) {
			c := chat.c
			if c.Tool == r.Tool && c.ID == r.ID && chat.host == r.tab.host &&
				*c.process == *r.tab.c.process && c.placed.process == r.tab.c.placed.process && stillRuns(c, s.Processes) {
				return c, nil
			}
		}
	}
	return Conversation{}, errTabMoved
}

// savedName is the name the marks file holds for one conversation.
func (s *Scanner) savedName(tool Tool, id string) (string, error) {
	file, _, err := readMarks(s.MarksPath, s.Now())
	if err != nil {
		return "", err
	}
	return file.Marks[markKey(tool, id)].Title, nil
}
