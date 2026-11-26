package conversations

import (
	"context"
	"errors"
)

// RenameInTool writes a name into the tool that keeps a conversation.
type RenameInTool func(ctx context.Context, c Conversation, title string) error

// ErrConversationGone says the list no longer holds the conversation.
var ErrConversationGone = errors.New("the conversation is not listed any more")

// errRenamedMeanwhile says the conversation's saved name changed after
// the caller read it, so the caller's name is older than the saved one.
var errRenamedMeanwhile = errors.New("the conversation was renamed meanwhile")

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
	// Saved, when set, is the name the conversation must still have. A
	// rename that finds another one changes nothing.
	Saved *string
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
	defer release()
	c, ok := s.Find(ctx, r.Tool, r.ID, r.Opts)
	if !ok {
		return Conversation{}, ErrConversationGone
	}
	if r.Check != nil {
		if err := r.Check(c); err != nil {
			return c, err
		}
	}
	if r.Saved != nil {
		saved, err := s.savedName(r.Tool, r.ID)
		if err != nil {
			return c, err
		}
		if saved != *r.Saved {
			return c, errRenamedMeanwhile
		}
	}
	if err := r.InTool(ctx, c, r.Title); err != nil {
		return c, &ToolRenameError{Err: err}
	}
	return c, s.Rename(r.Tool, r.ID, r.Title)
}

// savedName is the name the marks file holds for one conversation.
func (s *Scanner) savedName(tool Tool, id string) (string, error) {
	file, _, err := readMarks(s.MarksPath, s.Now())
	if err != nil {
		return "", err
	}
	return file.Marks[markKey(tool, id)].Title, nil
}
