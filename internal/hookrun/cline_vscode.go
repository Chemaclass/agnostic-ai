package hookrun

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

const ClineVSCodeTimeout = 30 * time.Second

func BuildClineVSCodeTaskStart(root string, in Input) (Payload, error) {
	if in.Raw != nil {
		return Payload{Body: in.Raw, Fires: true, Trigger: "task"}, nil
	}
	if in.Edit != "" || in.Bash != "" {
		return Payload{}, fmt.Errorf("--edit and --bash build tool events, not TaskStart")
	}
	if in.Prompt != "" {
		return Payload{}, fmt.Errorf("--prompt builds UserPromptSubmit, not TaskStart")
	}
	return clineMarshal(Payload{Fires: true, Trigger: "task"}, map[string]any{
		"clineVersion": "", "hookName": "TaskStart", "timestamp": strconv.FormatInt(time.Now().UnixMilli(), 10),
		"workspaceRoots": []string{root}, "userId": "", "taskId": SessionID,
		"model":     map[string]string{"provider": "unknown", "slug": "unknown"},
		"taskStart": map[string]any{"taskMetadata": map[string]string{"taskId": SessionID, "ulid": "", "initialTask": ""}},
	})
}

func ClineVSCodeTaskStartResult(r Result) (Decision, bool, []string) {
	if r.TimedOut {
		return Timeout, false, []string{"Cline VS Code logs a failed TaskStart hook and continues without its context"}
	}
	if r.StartErr != nil {
		return Error, false, []string{"Cline VS Code logs a failed TaskStart hook and continues without its context"}
	}
	reply := clineVSCodeReply(r.Stdout)
	if reply == nil {
		if r.Exit != 0 {
			return Error, false, []string{"Cline VS Code logs a failed TaskStart hook and continues without its context"}
		}
		return Allow, false, []string{"Cline VS Code found no valid hook reply and continues without context"}
	}
	if reply["cancel"] == true {
		return Block, false, nil
	}
	text, _ := reply["contextModification"].(string)
	return Allow, len(utf16.Encode([]rune(text))) > 50000 || strings.TrimSpace(text) != "", nil
}

func clineVSCodeReply(stdout string) map[string]any {
	parse := func(text string) map[string]any {
		var reply map[string]any
		if json.Unmarshal([]byte(text), &reply) != nil || reply == nil {
			return nil
		}
		if _, removed := reply["shouldContinue"]; removed {
			return nil
		}
		for key, value := range reply {
			switch key {
			case "cancel":
				if _, ok := value.(bool); !ok {
					return nil
				}
			case "contextModification", "errorMessage":
				if _, ok := value.(string); !ok {
					return nil
				}
			}
		}
		return reply
	}
	if reply := parse(stdout); reply != nil {
		return reply
	}
	lines := strings.Split(stdout, "\n")
	candidate, braces, collecting := "", 0, false
	for i := len(lines) - 1; i >= 0; i-- {
		for j := len(lines[i]) - 1; j >= 0; j-- {
			switch lines[i][j] {
			case '}':
				braces++
				collecting = true
			case '{':
				braces--
			}
		}
		if collecting {
			candidate = lines[i] + "\n" + candidate
		}
		if collecting && braces == 0 {
			break
		}
	}
	if at := strings.IndexByte(candidate, '{'); at >= 0 {
		return parse(strings.TrimSpace(candidate[at:]))
	}
	return nil
}
