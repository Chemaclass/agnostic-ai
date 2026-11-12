package conversations

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// cursorPromptsQueryWithoutReplies is cursorPromptsQuery before it read
// replies: the baseline a batch should stay close to.
const cursorPromptsQueryWithoutReplies = `WITH chosen(composerId) AS (VALUES %s),
headers AS (
  SELECT c.composerId, '' AS chunk, CAST(h.key AS INTEGER) AS seq, json_extract(h.value, '$.bubbleId') AS bubbleId
  FROM chosen c
  JOIN cursorDiskKV d ON d.key = 'composerData:' || c.composerId,
       json_each(d.value, '$.fullConversationHeadersOnly') h
  WHERE json_extract(h.value, '$.type') = 1
  UNION ALL
  SELECT c.composerId, k.key AS chunk, CAST(h.key AS INTEGER) AS seq, json_extract(h.value, '$.bubbleId') AS bubbleId
  FROM chosen c
  JOIN cursorDiskKV k ON k.key >= 'composerConvHeaders:' || c.composerId || ':'
                     AND k.key <  'composerConvHeaders:' || c.composerId || ';',
       json_each(k.value) h
  WHERE json_extract(h.value, '$.type') = 1
),
ends AS (
  SELECT composerId,
    (SELECT bubbleId FROM headers h2 WHERE h2.composerId = h.composerId ORDER BY chunk, seq LIMIT 1) AS firstBubble,
    (SELECT bubbleId FROM headers h2 WHERE h2.composerId = h.composerId ORDER BY chunk DESC, seq DESC LIMIT 1) AS lastBubble
  FROM headers h GROUP BY composerId
)
SELECT e.composerId AS id,
  coalesce(json_extract(fb.value, '$.text'), '') AS first_text,
  coalesce(json_extract(lb.value, '$.text'), '') AS last_text
FROM ends e
LEFT JOIN cursorDiskKV fb ON fb.key = 'bubbleId:' || e.composerId || ':' || e.firstBubble
LEFT JOIN cursorDiskKV lb ON lb.key = 'bubbleId:' || e.composerId || ':' || e.lastBubble`

// cursorPromptsQueryBySubquery read replies with a subquery per chat
// and end, which runs five times as long as the baseline on long chats.
var cursorPromptsQueryBySubquery = `WITH chosen(composerId) AS (VALUES %s),
headers AS (
  SELECT c.composerId, '' AS chunk, CAST(h.key AS INTEGER) AS seq, json_extract(h.value, '$.type') AS type, json_extract(h.value, '$.bubbleId') AS bubbleId
  FROM chosen c
  JOIN cursorDiskKV d ON d.key = 'composerData:' || c.composerId,
       json_each(d.value, '$.fullConversationHeadersOnly') h
  WHERE json_extract(h.value, '$.type') = 1 OR (json_extract(h.value, '$.type') = 2 AND coalesce(json_extract(h.value, '$.grouping.hasText'), json_type(h.value, '$.grouping') IS NULL))
  UNION ALL
  SELECT c.composerId, k.key AS chunk, CAST(h.key AS INTEGER) AS seq, json_extract(h.value, '$.type') AS type, json_extract(h.value, '$.bubbleId') AS bubbleId
  FROM chosen c
  JOIN cursorDiskKV k ON k.key >= 'composerConvHeaders:' || c.composerId || ':'
                     AND k.key <  'composerConvHeaders:' || c.composerId || ';',
       json_each(k.value) h
  WHERE json_extract(h.value, '$.type') = 1 OR (json_extract(h.value, '$.type') = 2 AND coalesce(json_extract(h.value, '$.grouping.hasText'), json_type(h.value, '$.grouping') IS NULL))
),
prompts AS (
  SELECT composerId,
    (SELECT bubbleId FROM headers h2 WHERE h2.composerId = h.composerId AND h2.type = 1 ORDER BY chunk, seq LIMIT 1) AS firstBubble,
    (SELECT bubbleId FROM headers h2 WHERE h2.composerId = h.composerId AND h2.type = 1 ORDER BY chunk DESC, seq DESC LIMIT 1) AS lastBubble,
    (SELECT chunk FROM headers h2 WHERE h2.composerId = h.composerId AND h2.type = 1 ORDER BY chunk DESC, seq DESC LIMIT 1) AS lastChunk,
    (SELECT seq FROM headers h2 WHERE h2.composerId = h.composerId AND h2.type = 1 ORDER BY chunk DESC, seq DESC LIMIT 1) AS lastSeq
  FROM headers h GROUP BY composerId
),
ends AS (
  SELECT p.*,
    (SELECT json_extract(b.value, '$.text') FROM headers h2
     JOIN cursorDiskKV b ON b.key = 'bubbleId:' || h2.composerId || ':' || h2.bubbleId
     WHERE h2.composerId = p.composerId AND h2.type = 2
       AND (p.lastBubble IS NULL OR h2.chunk > p.lastChunk OR (h2.chunk = p.lastChunk AND h2.seq > p.lastSeq))
       AND coalesce(json_extract(b.value, '$.text'), '') <> ''
     ORDER BY h2.chunk DESC, h2.seq DESC LIMIT 1) AS reply
  FROM prompts p
)
SELECT e.composerId AS id,
  coalesce(json_extract(fb.value, '$.text'), '') AS first_text,
  coalesce(json_extract(lb.value, '$.text'), '') AS last_text,
  ` + replyEndsColumns(`coalesce(e.reply, '')`) + `
FROM ends e
LEFT JOIN cursorDiskKV fb ON fb.key = 'bubbleId:' || e.composerId || ':' || e.firstBubble
LEFT JOIN cursorDiskKV lb ON lb.key = 'bubbleId:' || e.composerId || ':' || e.lastBubble`

// cursorBenchStore writes a Cursor store of chats with headers message
// headers each, inline, every third one a prompt and every third one an
// agent message with text. It returns the store and the chosen list.
func cursorBenchStore(b *testing.B, chats, headers int) (path, chosen string) {
	b.Helper()
	if _, err := exec.LookPath("sqlite3"); err != nil {
		b.Skip("sqlite3 is not installed")
	}
	path = filepath.Join(b.TempDir(), "state.vscdb")
	text := strings.Repeat("The cookie was dropped on the redirect, so I changed SameSite and added a test. ", 3)
	sql := fmt.Sprintf(`CREATE TABLE cursorDiskKV (key TEXT UNIQUE ON CONFLICT REPLACE, value BLOB);
WITH RECURSIVE c(n) AS (SELECT 0 UNION ALL SELECT n + 1 FROM c WHERE n + 1 < %[1]d),
i(n) AS (SELECT 0 UNION ALL SELECT n + 1 FROM i WHERE n + 1 < %[2]d)
INSERT INTO cursorDiskKV SELECT printf('bubbleId:chat%%05d:b%%d', c.n, i.n),
  json_object('type', CASE i.n %% 3 WHEN 0 THEN 1 ELSE 2 END, 'text', CASE i.n %% 3 WHEN 1 THEN '' ELSE '%[3]s' END)
FROM c, i;
WITH RECURSIVE c(n) AS (SELECT 0 UNION ALL SELECT n + 1 FROM c WHERE n + 1 < %[1]d),
i(n) AS (SELECT 0 UNION ALL SELECT n + 1 FROM i WHERE n + 1 < %[2]d)
INSERT INTO cursorDiskKV SELECT printf('composerData:chat%%05d', c.n),
  json_object('fullConversationHeadersOnly', json_group_array(json(CASE i.n %% 3
    WHEN 0 THEN json_object('bubbleId', 'b' || i.n, 'type', 1, 'grouping', json_object('hasText', json('true')))
    WHEN 1 THEN json_object('bubbleId', 'b' || i.n, 'type', 2, 'grouping', json_object('capabilityType', 15))
    ELSE json_object('bubbleId', 'b' || i.n, 'type', 2, 'grouping', json_object('hasText', json('true'))) END)))
FROM c, i GROUP BY c.n;`, chats, headers, text)
	if out, err := exec.Command("sqlite3", path, sql).CombinedOutput(); err != nil {
		b.Fatalf("sqlite3: %v: %s", err, out)
	}
	ids := make([]string, chats)
	for n := range ids {
		ids[n] = fmt.Sprintf("('chat%05d')", n)
	}
	return path, strings.Join(ids, ",")
}

// BenchmarkCursorPromptsQuery times one batch of cursorPromptBatch chats
// through the system sqlite3: the query before replies (baseline), the
// first query with replies (subqueries), and the one in use (ranked).
func BenchmarkCursorPromptsQuery(b *testing.B) {
	for _, headers := range []int{300, 900} {
		path, chosen := cursorBenchStore(b, cursorPromptBatch, headers)
		for _, subject := range []struct{ name, query string }{
			{"baseline", cursorPromptsQueryWithoutReplies},
			{"subqueries", cursorPromptsQueryBySubquery},
			{"ranked", cursorPromptsQuery},
		} {
			b.Run(fmt.Sprintf("%s/%d_headers", subject.name, headers), func(b *testing.B) {
				query := fmt.Sprintf(subject.query, chosen)
				for b.Loop() {
					rows, err := RunSQLite(context.Background(), path, query)
					if err != nil || len(rows) != cursorPromptBatch {
						b.Fatalf("%d rows: %v", len(rows), err)
					}
				}
			})
		}
	}
}

// benchSearchChats is a heavy Cursor user's editor chat count.
const benchSearchChats = 5000

// benchCursorPrompts is what the prompts query returns for one chat:
// wrapped prompts and a long reply cut to its two ends.
func benchCursorPrompts(n int) cursorPrompts {
	prompt := func(text string) string {
		return "<timestamp>Friday, Nov 7, 2025, 8:00 AM</timestamp>\n<user_query>" + text + "</user_query>"
	}
	reply := strings.Repeat("The cookie was dropped on the redirect, so I changed SameSite and added a test. ", 60)
	return cursorPrompts{
		ID:    fmt.Sprintf("chat%05d", n),
		First: prompt(fmt.Sprintf("Fix the login bug %d", n)),
		Last:  prompt("Now add a test for the redirect"),
		replyEnds: replyEnds{
			Start:  reply[:replyCheckedBytes+1],
			Ending: reply[len(reply)-replyCheckedBytes-1:] + "\n\nShould I also update the docs?",
			Length: len(reply) + 32,
		},
	}
}

// BenchmarkCursorSearchTexts times the part of a search that turns each
// pending editor chat's cached store texts into the texts a prefilter
// reads, over that many chats: from the raw ends the cache holds now
// (status quo), from texts computed once when cached (proposed), and
// from the store with no cache (uncached; its texts are shorter).
func BenchmarkCursorSearchTexts(b *testing.B) {
	words := queryWords("redirect docs")
	raw := map[string]cursorPrompts{}
	computed := map[string]texts{}
	ids := make([]string, benchSearchChats)
	for n := range ids {
		row := benchCursorPrompts(n)
		ids[n] = row.ID
		raw[row.ID] = row
		computed[row.ID] = withPrompts(texts{}, row)
	}
	c := Conversation{Tool: ToolCursor, App: AppIDE, Folder: "/tmp/app"}
	search := func(t texts) bool {
		return holdsAll(searchText(c, t.title, t.firstPrompt+"\n"+t.lastPrompt, replySearched(t)), words)
	}
	b.Run("status_quo", func(b *testing.B) {
		for b.Loop() {
			for _, id := range ids {
				search(withPrompts(texts{title: "Chat"}, raw[id]))
			}
		}
	})
	b.Run("proposed", func(b *testing.B) {
		for b.Loop() {
			for _, id := range ids {
				t := computed[id]
				t.title = "Chat"
				search(t)
			}
		}
	})
	b.Run("uncached", func(b *testing.B) {
		path, _ := cursorBenchStore(b, benchSearchChats, 30)
		for b.Loop() {
			for start := 0; start < len(ids); start += cursorPromptBatch {
				batch := ids[start:min(start+cursorPromptBatch, len(ids))]
				values := make([]string, len(batch))
				for i, id := range batch {
					values[i] = "('" + id + "')"
				}
				rows, err := queryRows[cursorPrompts](context.Background(), RunSQLite, path, fmt.Sprintf(cursorPromptsQuery, strings.Join(values, ",")))
				if err != nil || len(rows) != len(batch) {
					b.Fatalf("%d rows: %v", len(rows), err)
				}
				for _, row := range rows {
					search(withPrompts(texts{title: "Chat"}, row))
				}
			}
		}
	})
}
