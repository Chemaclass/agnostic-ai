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

func TestReportUnsupported_OneLinePerTargetKindWithCount(t *testing.T) {
	buf := swapWarner(t)
	caps := Capabilities{Target: "aider", Supports: []spec.Kind{spec.KindRule}}
	b := spec.Bundle{
		Hooks: []spec.Entry{{Name: "h1"}, {Name: "h2"}, {Name: "h3"}, {Name: "h4"}},
	}
	if err := ReportUnsupported(caps, b, OnUnsupportedWarn); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "aider: 4 hooks skipped") {
		t.Errorf("expected aggregate count in warning, got:\n%s", got)
	}
	if strings.Count(got, "hooks skipped") != 1 {
		t.Errorf("expected exactly one hooks warning line, got:\n%s", got)
	}
}

func TestReportUnsupported_DedupAcrossInvocations(t *testing.T) {
	buf := swapWarner(t)
	caps := Capabilities{Target: "aider", Supports: []spec.Kind{spec.KindRule}}
	b := spec.Bundle{Hooks: []spec.Entry{{Name: "h1"}}}
	for i := 0; i < 3; i++ {
		if err := ReportUnsupported(caps, b, OnUnsupportedWarn); err != nil {
			t.Fatal(err)
		}
	}
	got := buf.String()
	if strings.Count(got, "hook skipped") != 1 {
		t.Errorf("expected single warning line across multiple Emit calls, got:\n%s", got)
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
	if err := ReportUnsupported(caps, b, OnUnsupportedWarn); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if strings.Count(got, "on-unsupported: silent") != 1 {
		t.Errorf("expected suppression hint exactly once, got:\n%s", got)
	}
}

func TestReportUnsupported_PluralizesCorrectly(t *testing.T) {
	buf := swapWarner(t)
	caps := Capabilities{Target: "aider", Supports: []spec.Kind{spec.KindRule}}
	b := spec.Bundle{Hooks: []spec.Entry{{Name: "h1"}}}
	if err := ReportUnsupported(caps, b, OnUnsupportedWarn); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "1 hook skipped") {
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
	if buf.Len() != 0 {
		t.Errorf("silent mode must not write to Warner, got: %s", buf.String())
	}
}
