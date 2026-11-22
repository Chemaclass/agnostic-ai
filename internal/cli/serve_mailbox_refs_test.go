package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestMailboxHTTP_ReadsReviewRefsFromTheSelectedRowsCurrentWorktree(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	base, commit := makeReviewRepo(t, h.root)
	source := filepath.Join(h.root, "src")
	testutil.Chdir(t, source)
	id := fmt.Sprintf(`{"tool":"claude","id":%q}`, inProject)
	response := mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/refs", id)
	if response.Code != http.StatusOK {
		t.Fatalf("review refs = %d %s", response.Code, response.Body)
	}
	refs := parseReviewRefsReply(t, response.Body.Bytes())
	if !sameRealDir(h.root, refs.Worktree) || refs.Base != base || refs.Commit != commit ||
		len(refs.Paths) != 1 || refs.Paths[0] != "src/parser.go" || refs.Uncommitted != 0 {
		t.Errorf("review refs = %#v", refs)
	}
	out, err := runRoot(t, "mailbox", "refs")
	if err != nil || strings.TrimSpace(out) != strings.TrimSpace(response.Body.String()) {
		t.Errorf("CLI and HTTP differed: CLI = %s (%v), HTTP = %s", out, err, response.Body)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(h.scanner.MarksPath), "mailbox")); !os.IsNotExist(err) {
		t.Errorf("refs preview created mailbox state: %v", err)
	}
}

func reviewSendBody(worktree, base, commit string) string {
	return fmt.Sprintf(`{"from":{"tool":"claude","id":%q},"to":{"id":%q},"type":"review_request","refs":{"worktree":%q,"base":%q,"commit":%q}}`,
		inProject, inWorktree, worktree, base, commit)
}

func TestMailboxHTTP_SendsReviewRefsFromTheSenderButRefusesAnotherWorktree(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	base, commit := makeReviewRepo(t, h.root)
	other := realDir(t)
	otherBase, otherCommit := makeReviewRepo(t, other)
	body := reviewSendBody(other, otherBase, otherCommit)
	mailboxExpectRefusal(t, mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/send", body), "refs")

	body = reviewSendBody(h.root, base, commit)
	body = strings.TrimSuffix(body, "}") + `,"models":{"from":"gpt-6.2-sol","to":"gpt-6.2-astra"}}`
	mailboxExpectRefusal(t, mailboxHTTPAs(t, h, "", http.MethodPost, "/api/mailbox/send", body), "format")
	id := mailboxSendThread(t, h, body)
	rec := mailboxHTTP(t, h, http.MethodGet, "/api/mailbox/threads?tool=claude&id="+inProject, "")
	var threads struct {
		Threads []struct {
			ID       string
			Messages []struct {
				Type string
				Refs reviewRefsReply
				Body *string
			}
			Models map[string]struct{ Name, StatedBy string }
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &threads); err != nil || len(threads.Threads) != 1 || len(threads.Threads[0].Messages) != 1 {
		t.Fatalf("stored review request = %d %s (%v)", rec.Code, rec.Body, err)
	}
	thread := threads.Threads[0]
	message := thread.Messages[0]
	if thread.ID != id || message.Type != "review_request" || message.Body != nil ||
		message.Refs.Base != base || message.Refs.Commit != commit || len(message.Refs.Paths) != 1 {
		t.Errorf("stored review request = %#v", thread)
	}
	model := thread.Models["claude:"+inProject]
	if model.Name != "gpt-6.2-sol" || model.StatedBy != "human" {
		t.Errorf("source model = %#v", model)
	}
}

func TestMailboxHTTP_RefusesVerdictFieldsAndAmbiguousJSONOnReviewRequests(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	body := reviewSendBody(h.root, "main", "HEAD")
	for _, test := range []struct{ name, body string }{
		{"empty body", strings.TrimSuffix(body, "}") + `,"body":""}`},
		{"null body", strings.TrimSuffix(body, "}") + `,"body":null}`},
		{"conclusion", strings.TrimSuffix(body, "}") + `,"conclusion":"verdict not in audit"}`},
		{"verdict", strings.TrimSuffix(body, "}") + `,"verdict":"verdict not in audit"}`},
		{"unknown field", strings.TrimSuffix(body, "}") + `,"summary":"verdict not in audit"}`},
		{"unknown ref field", strings.Replace(body, `"worktree":`, `"branch":"approved","worktree":`, 1)},
		{"unknown model field", strings.TrimSuffix(body, "}") + `,"models":{"from":"model","remark":"approved"}}`},
		{"forged sender", strings.Replace(body, `"from":{"tool":`, `"from":{"by":"agent","tool":`, 1)},
		{"forged source folder", strings.TrimSuffix(body, "}") + `,"reviewFolder":"/another/worktree"}`},
		{"case alias", strings.Replace(body, `"type":`, `"TYPE":`, 1)},
		{"duplicate type", strings.Replace(body, `"type":"review_request"`, `"type":"review_request","type":"question"`, 1)},
		{"duplicate nested key", strings.Replace(body, `"base":"main"`, `"base":"main","base":"main"`, 1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			mailboxExpectRefusal(t, mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/send", test.body), "echo")
		})
	}
	audit, err := os.ReadFile(filepath.Join(filepath.Dir(h.scanner.MarksPath), "mailbox", "audit.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(audit), "verdict not in audit") || !strings.Contains(string(audit), `"guard": "echo"`) {
		t.Errorf("review refusal audit = %s", audit)
	}
}

func TestMailboxHTTP_ReviewRefInputRejectsACredential(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	base, commit := makeReviewRepo(t, h.root)
	body := reviewSendBody(h.root, base, commit)
	body = strings.Replace(body, `"worktree":`, `"paths":["password=fixture-telemetry-example"],"worktree":`, 1)
	rec := mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/send", body)
	mailboxExpectRefusal(t, rec, "secret")
	if strings.Contains(rec.Body.String(), "fixture-telemetry-example") {
		t.Errorf("review send leaked the credential: %s", rec.Body)
	}
}
