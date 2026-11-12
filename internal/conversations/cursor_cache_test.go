package conversations

import (
	"context"
	"encoding/json"
	"testing"
)

func TestCursorEditorPrompts_RereadsAChatWhoseRecencyMovesWithinASecond(t *testing.T) {
	calls := 0
	s := &Scanner{SQLite: func(_ context.Context, _, _ string) ([]json.RawMessage, error) {
		calls++
		prompt := "Before"
		if calls > 1 {
			prompt = "After"
		}
		return []json.RawMessage{json.RawMessage(`{"id":"aaaaaaaa","first_text":"First","last_text":"` + prompt + `"}`)}, nil
	}}
	row := Conversation{ID: "aaaaaaaa", App: AppIDE, UpdatedAt: timeFromMillis(1762500000100), recency: 1762500000100}
	before := s.cursorEditorPrompts(context.Background(), []Conversation{row})[row.ID]
	row.UpdatedAt, row.recency = timeFromMillis(1762500000900), 1762500000900
	after := s.cursorEditorPrompts(context.Background(), []Conversation{row})[row.ID]
	if after.lastPrompt != "After" {
		t.Fatalf("before=%q after=%q queries=%d updatedAt=%q; a new recency in the same second must be read again", before.lastPrompt, after.lastPrompt, calls, row.UpdatedAt)
	}
}
