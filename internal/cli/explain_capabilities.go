package cli

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

type explainCapability struct {
	Target     string   `json:"target"`
	Field      string   `json:"field"`
	Capability string   `json:"capability"`
	Native     []string `json:"native"`
	Supported  bool     `json:"supported"`
	Widening   []string `json:"widening,omitempty"`
	Override   string   `json:"override,omitempty"`
}

func explainCapabilities(e spec.Entry, cfg *config.Config, settings ...spec.Entry) []explainCapability {
	if len(settings) == 0 && e.Kind == spec.KindSettings {
		settings = []spec.Entry{e}
	}
	var result []explainCapability
	for _, target := range slices.Sorted(slices.Values(cfg.Targets)) {
		if !e.EmitsTo(target) {
			continue
		}
		switch e.Kind {
		case spec.KindAgent:
			field := "tools"
			if _, set := e.Meta["can"]; set {
				field = "can"
			}
			value := e.Meta[field]
			override, _ := e.Meta["x-"+target].(map[string]any)
			if native, set := override["tools"]; set {
				rules := explainToolList(native)
				if len(rules) == 0 {
					result = append(result, capabilityExplanation(target, "x-"+target+".tools", "native configuration", adapters.TranslateAgentCapabilityIn(target, "", e)))
				} else {
					for _, rule := range rules {
						translated := adapters.TranslateAgentCapabilityIn(target, rule, e)
						if translated.Supported {
							translated.Native = []string{rule}
						}
						result = append(result, capabilityExplanation(target, "x-"+target+".tools", rule, translated))
					}
				}
				continue
			}
			for _, rule := range explainToolList(value) {
				result = append(result, capabilityExplanation(target, field, rule, adapters.TranslateAgentCapabilityIn(target, rule, e)))
			}
		case spec.KindSettings:
			permissions, _ := e.Meta["permissions"].(map[string]any)
			for _, list := range spec.PermissionLists {
				for _, rule := range toStringSlice(permissions[list]) {
					result = append(result, capabilityExplanation(target, "permissions."+list, rule, adapters.TranslatePermissionCapabilityIn(target, list, rule, e, settings, cfg)))
				}
			}
		}
	}
	return result
}

func explainToolList(value any) []string {
	if scalar, ok := value.(string); ok {
		return spec.SplitToolList(scalar)
	}
	return toStringSlice(value)
}

func capabilityExplanation(target, field, rule string, translated adapters.CapabilityTranslation) explainCapability {
	native := translated.Native
	if native == nil {
		native = []string{}
	}
	return explainCapability{Target: target, Field: field, Capability: rule, Native: native, Supported: translated.Supported, Widening: translated.Widening, Override: translated.Override}
}

func printExplainCapabilities(out io.Writer, capabilities []explainCapability) {
	if len(capabilities) == 0 {
		return
	}
	_, _ = fmt.Fprintln(out, "\ncapabilities:")
	for _, capability := range capabilities {
		native := strings.Join(capability.Native, ", ")
		if !capability.Supported {
			if native != "" {
				native += " (partly unsupported)"
			} else {
				native = "unsupported"
			}
		}
		_, _ = fmt.Fprintf(out, "  [%s] %s: %s → %s\n", capability.Target, capability.Field, capability.Capability, native)
		if capability.Override != "" {
			_, _ = fmt.Fprintf(out, "    override: %s\n", capability.Override)
		}
		for _, widening := range capability.Widening {
			_, _ = fmt.Fprintf(out, "    widening: %s\n", widening)
		}
	}
}
