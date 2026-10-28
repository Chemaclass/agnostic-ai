package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestEditField_AudienceAndLegacyScopeWriteTheSameFile(t *testing.T) {
	project := scopeProject(t)
	h := newEditHandler(project, false)
	shared := ".agnostic-ai/settings/defaults.yaml"
	overlay := ".agnostic-ai/local/settings/defaults.yaml"
	before := readOptional(t, shared)
	for _, tc := range []struct{ fields, model string }{
		{`"audience":"personal"`, "opus"},
		{`"scope":"local"`, "haiku"},
		{`"audience":"personal","scope":"local"`, "sonnet-4"},
	} {
		rec := postEdit(t, h, editFieldPath, `{`+tc.fields+`,"path":"`+shared+`","key":"model","value":"`+tc.model+`"}`, nil)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), overlay) {
			t.Fatalf("%s: save = %d %s", tc.fields, rec.Code, rec.Body)
		}
		if got := readOptional(t, overlay); !strings.Contains(got, "model: "+tc.model+"\n") {
			t.Errorf("%s: overlay = %q", tc.fields, got)
		}
	}
	if got := readOptional(t, shared); got != before {
		t.Errorf("shared spec changed:\n%s", got)
	}

	rec := postEdit(t, h, editFieldPath, `{"audience":"shared","path":"`+shared+`","key":"effort","value":"low"}`, nil)
	if rec.Code != http.StatusOK || !strings.Contains(readOptional(t, shared), "effort: low\n") {
		t.Fatalf("shared save = %d %s\n%s", rec.Code, rec.Body, readOptional(t, shared))
	}
}

func TestEditBuiltin_AudiencePersonalWritesThePersonalConfig(t *testing.T) {
	project := scopeProject(t)
	h := newEditHandler(project, false)
	before := readOptional(t, "agnostic-ai.yaml")
	rec := postEdit(t, h, editBuiltinPath, `{"audience":"personal","name":"handoff","enabled":true}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("save = %d %s", rec.Code, rec.Body)
	}
	if got := readOptional(t, "agnostic-ai.local.yaml"); !strings.Contains(got, "handoff") {
		t.Errorf("personal config = %q", got)
	}
	if got := readOptional(t, "agnostic-ai.yaml"); got != before {
		t.Errorf("shared config changed:\n%s", got)
	}
}

func TestEdit_RefusesAudienceAndScopeThatDisagree(t *testing.T) {
	project := scopeProject(t)
	edits := newEditHandler(project, false)
	changes := newBuiltinChangeHandler(project, false)
	for _, tc := range []struct {
		h          http.Handler
		path, body string
	}{
		{edits, editFieldPath, `{"audience":"shared","scope":"local","key":"model","value":"opus"}`},
		{edits, editFieldPath, `{"audience":"personal","scope":"team","key":"model","value":"opus"}`},
		{edits, editBuiltinPath, `{"audience":"shared","scope":"local","name":"handoff","enabled":true}`},
		{edits, editValuePath, `{"audience":"personal","scope":"team","path":".agnostic-ai/settings/defaults.yaml","key":"x","value":true}`},
		{changes, builtinPreviewPath, `{"audience":"shared","scope":"local","name":"caveman","enabled":true}`},
	} {
		rec := postEdit(t, tc.h, tc.path, tc.body, nil)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "disagree") {
			t.Errorf("%s %s = %d %s", tc.path, tc.body, rec.Code, rec.Body)
		}
	}
}

func TestEdit_RefusesAnUnknownAudience(t *testing.T) {
	project := scopeProject(t)
	for _, tc := range []struct {
		h          http.Handler
		path, body string
	}{
		{newEditHandler(project, false), editFieldPath, `{"audience":"team","key":"model","value":"opus"}`},
		{newBuiltinChangeHandler(project, false), builtinPreviewPath, `{"audience":"local","name":"caveman","enabled":true}`},
	} {
		rec := postEdit(t, tc.h, tc.path, tc.body, nil)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `audience must be \"shared\" or \"personal\"`) {
			t.Errorf("%s %s = %d %s", tc.path, tc.body, rec.Code, rec.Body)
		}
	}
}

func TestBuiltinChanges_PreviewEchoesAudienceAndLegacyScope(t *testing.T) {
	builtinChangeProject(t)
	h := newBuiltinChangeHandler(".", false)
	for _, tc := range []struct{ body, audience, scope string }{
		{`{"audience":"personal","name":"caveman","enabled":true}`, "personal", "local"},
		{`{"scope":"team","name":"caveman","enabled":true}`, "shared", "team"},
	} {
		rec := postEdit(t, h, builtinPreviewPath, tc.body, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: preview = %d %s", tc.body, rec.Code, rec.Body)
		}
		var view builtinChangeView
		if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
			t.Fatal(err)
		}
		if view.Audience != tc.audience || view.Scope != tc.scope {
			t.Errorf("%s: audience %q scope %q, want %q %q", tc.body, view.Audience, view.Scope, tc.audience, tc.scope)
		}
		builtinAction(t, h, builtinCancelPath, view.ID)
	}
	if rec := postEdit(t, h, builtinApplyPath, `{"id":"x","audience":"shared"}`, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("apply with an audience = %d %s", rec.Code, rec.Body)
	}
}
