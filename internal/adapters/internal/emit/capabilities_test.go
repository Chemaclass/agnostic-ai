package emit

import (
	"bytes"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func swapWarner(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := Warner
	Warner = buf
	t.Cleanup(func() { Warner = prev })
	ResetCapabilityWarnings()
	t.Cleanup(ResetCapabilityWarnings)
	return buf
}

func TestReportUnsupported_FlushGroupsByKind(t *testing.T) {
	buf := swapWarner(t)
	b := spec.Bundle{
		Hooks: []spec.Entry{{Name: "h1"}, {Name: "h2"}, {Name: "h3"}, {Name: "h4"}},
	}
	for _, target := range []string{"aider", "cline", "cursor"} {
		caps := Capabilities{Target: target, Supports: []spec.Kind{spec.KindRule}}
		if err := ReportUnsupported(caps, b, OnUnsupportedWarn); err != nil {
			t.Fatal(err)
		}
	}
	if buf.Len() != 0 {
		t.Fatalf("warnings must buffer until flush, got early output: %s", buf)
	}
	FlushCapabilityWarnings()
	got := buf.String()
	want := "  ! 4 hooks unsupported by aider, cline, cursor\n"
	if !strings.Contains(got, want) {
		t.Errorf("expected grouped line %q, got:\n%s", want, got)
	}
	if strings.Count(got, "hooks unsupported") != 1 {
		t.Errorf("expected exactly one hooks line after flush, got:\n%s", got)
	}
}

func TestReportUnsupported_DedupTargetKindWithinFlush(t *testing.T) {
	buf := swapWarner(t)
	caps := Capabilities{Target: "aider", Supports: []spec.Kind{spec.KindRule}}
	b := spec.Bundle{Hooks: []spec.Entry{{Name: "h1"}}}
	for i := 0; i < 3; i++ {
		if err := ReportUnsupported(caps, b, OnUnsupportedWarn); err != nil {
			t.Fatal(err)
		}
	}
	FlushCapabilityWarnings()
	got := buf.String()
	if strings.Count(got, "hook unsupported") != 1 {
		t.Errorf("expected single warning line for repeat reports of same (target, kind), got:\n%s", got)
	}
}

func TestReportUnsupported_PrintsSuppressionHintOnce(t *testing.T) {
	buf := swapWarner(t)
	caps := Capabilities{Target: "aider", Supports: []spec.Kind{spec.KindRule}}
	b := spec.Bundle{
		Hooks:    []spec.Entry{{Name: "h1"}},
		Commands: []spec.Entry{{Name: "c1"}},
	}
	if err := ReportUnsupported(caps, b, OnUnsupportedWarn); err != nil {
		t.Fatal(err)
	}
	FlushCapabilityWarnings()
	if strings.Count(buf.String(), "on-unsupported: silent") != 1 {
		t.Errorf("expected suppression hint exactly once per flush, got:\n%s", buf.String())
	}
}

func TestReportUnsupported_PluralizesCorrectly(t *testing.T) {
	buf := swapWarner(t)
	caps := Capabilities{Target: "aider", Supports: []spec.Kind{spec.KindRule}}
	b := spec.Bundle{Hooks: []spec.Entry{{Name: "h1"}}}
	if err := ReportUnsupported(caps, b, OnUnsupportedWarn); err != nil {
		t.Fatal(err)
	}
	FlushCapabilityWarnings()
	if !strings.Contains(buf.String(), "1 hook unsupported by aider") {
		t.Errorf("expected singular 'hook' for n=1, got:\n%s", buf.String())
	}
}

func TestReportUnsupported_SilentSkipsAll(t *testing.T) {
	buf := swapWarner(t)
	caps := Capabilities{Target: "aider", Supports: []spec.Kind{spec.KindRule}}
	b := spec.Bundle{Hooks: []spec.Entry{{Name: "h1"}}}
	if err := ReportUnsupported(caps, b, OnUnsupportedSilent); err != nil {
		t.Fatal(err)
	}
	FlushCapabilityWarnings()
	if buf.Len() != 0 {
		t.Errorf("silent mode must produce no output, got: %s", buf.String())
	}
}

func TestReportUnsupported_FlushClearsBuffer(t *testing.T) {
	buf := swapWarner(t)
	caps := Capabilities{Target: "aider", Supports: []spec.Kind{spec.KindRule}}
	b := spec.Bundle{Hooks: []spec.Entry{{Name: "h1"}}}
	if err := ReportUnsupported(caps, b, OnUnsupportedWarn); err != nil {
		t.Fatal(err)
	}
	FlushCapabilityWarnings()
	first := buf.String()
	FlushCapabilityWarnings()
	if buf.String() != first {
		t.Errorf("second flush should be no-op, got extra output:\n%s", buf.String())
	}
}
