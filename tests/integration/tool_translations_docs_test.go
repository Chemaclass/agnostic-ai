package integration

import (
	"fmt"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const toolTranslationsMarker = "# Native tool names from the adapter capability tables."

func TestSiteDocs_ToolTranslationsMatchAdapters(t *testing.T) {
	want := adapters.ToolCapabilityMatrix()
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		data, err := os.ReadFile(capabilitiesData)
		if err != nil {
			t.Fatal(err)
		}
		before, _, _ := strings.Cut(string(data), toolTranslationsMarker)
		var section strings.Builder
		section.WriteString(toolTranslationsMarker + "\n")
		targets := make([]string, 0, len(want))
		for target := range want {
			targets = append(targets, target)
		}
		slices.Sort(targets)
		for _, target := range targets {
			fmt.Fprintf(&section, "\n[tool_translations.%s]\n", target)
			for _, capability := range spec.Capabilities {
				var names []string
				for _, name := range want[target][capability] {
					names = append(names, strconv.Quote(name))
				}
				fmt.Fprintf(&section, "%s = [%s]\n", capability, strings.Join(names, ", "))
			}
		}
		if err := os.WriteFile(capabilitiesData, []byte(strings.TrimRight(before, "\n")+"\n\n"+section.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var doc struct {
		ToolTranslations map[string]map[string][]string `toml:"tool_translations"`
	}
	if _, err := toml.DecodeFile(capabilitiesData, &doc); err != nil {
		t.Fatal(err)
	}
	for target, rows := range want {
		for _, capability := range spec.Capabilities {
			if rows[capability] == nil {
				rows[capability] = []string{}
			}
		}
		if !reflect.DeepEqual(doc.ToolTranslations[target], rows) {
			t.Errorf("%s native tool data = %v, want %v; regenerate with UPDATE_GOLDEN=1", target, doc.ToolTranslations[target], rows)
		}
	}
	if len(doc.ToolTranslations) != len(want) {
		t.Errorf("native tool tables = %d, want %d", len(doc.ToolTranslations), len(want))
	}
}
