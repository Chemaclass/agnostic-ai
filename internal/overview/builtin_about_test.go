package overview

import (
	"strings"
	"testing"
)

func TestBuiltinItems_ExplainEveryBuiltin(t *testing.T) {
	docs := map[string]string{
		"caveman": "https://github.com/JuliusBrussee/caveman",
		"rtk":     "https://github.com/rtk-ai/rtk",
	}
	for _, item := range builtinItems(Inputs{}) {
		if item.Description == "Built-in feature." || item.Label == item.Name {
			t.Errorf("%s description %q label %q, want its own line and label", item.Name, item.Description, item.Label)
		}
		about := item.About
		if about == nil || about.Summary == "" || len(about.Points) < 2 {
			t.Errorf("%s about = %+v, want a summary and at least two points", item.Name, about)
			continue
		}
		if about.DocsURL != docs[item.Name] {
			t.Errorf("%s docs = %q, want %q", item.Name, about.DocsURL, docs[item.Name])
		}
		if (about.DocsURL == "") != (about.DocsLabel == "") {
			t.Errorf("%s docs label = %q for url %q", item.Name, about.DocsLabel, about.DocsURL)
		}
		for _, text := range append([]string{about.Summary}, about.Points...) {
			if strings.ContainsAny(text, "\U00002013\U00002014") {
				t.Errorf("%s text %q has a long dash", item.Name, text)
			}
		}
	}
}

func TestBuiltinAbout_UnknownBuiltinHasNone(t *testing.T) {
	if builtinAbout("not-a-builtin") != nil {
		t.Error("unknown built-in got an about, want none")
	}
}
