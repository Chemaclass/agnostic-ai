package opencode

import (
	"syscall/js"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
)

func TestProjectPermissionBrowserReplaysPlannedRulesWithoutFilesystem(t *testing.T) {
	fs := js.Global().Get("fs")
	open := fs.Get("open")
	unavailable := js.FuncOf(func(_ js.Value, args []js.Value) any {
		err := js.Global().Get("Error").New("not implemented on js")
		err.Set("code", "ENOSYS")
		args[len(args)-1].Invoke(err)
		return nil
	})
	fs.Set("open", unavailable)
	defer func() {
		fs.Set("open", open)
		unavailable.Release()
	}()

	sess := emit.NewSession()
	sess.StartCapture()
	sess.WatchPlannedFile("opencode.json")
	if err := sess.WriteFile("opencode.json", `{"permissions":[{"action":"read","resource":"secrets/*","effect":"deny"}]}`, false); err != nil {
		t.Fatal(err)
	}
	project, err := readProjectPermission(sess, "opencode.json", false)
	if err != nil {
		t.Fatal(err)
	}
	if !project.native {
		t.Error("planned native permissions were not selected")
	}
	for _, rule := range project.rules {
		if rule == (permissionRule{"read", "secrets/*", "deny"}) {
			return
		}
	}
	t.Error("planned read restriction was not replayed")
}
