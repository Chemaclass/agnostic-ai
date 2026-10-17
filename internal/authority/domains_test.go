package authority

import (
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestMCPDomains_PlacesServersThroughTheCuratedTable(t *testing.T) {
	for _, tc := range []struct {
		name string
		meta map[string]any
		want []string
	}{
		{"github", map[string]any{"url": "https://api.githubcopilot.com/mcp/"}, []string{DomainSourceControl}},
		{"railway", map[string]any{"url": "https://mcp.railway.app/mcp"}, []string{DomainCloud}},
		{"analytics-db", map[string]any{"command": "npx", "args": []any{"-y", "@modelcontextprotocol/server-postgres", "${URL}"}}, []string{DomainData}},
		{"workspace-files", map[string]any{"command": "npx", "args": []any{"-y", "@modelcontextprotocol/server-filesystem", "/srv/db"}}, []string{DomainFilesystem}},
		{"assets", map[string]any{"url": "https://s3.amazonaws.com/x"}, []string{DomainCloud, DomainData}},
		{"team-chat", map[string]any{"command": "slack-mcp"}, []string{DomainCommunication}},
		{"acme-tool", map[string]any{"command": "acme"}, nil},
	} {
		got := mcpDomains(spec.Entry{Kind: spec.KindMCP, Name: tc.name, Meta: tc.meta})
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: domains = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestBuild_UnknownServerDefaultsToAgentOpsAndSaysSo(t *testing.T) {
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":              "version: 1\ntargets: [claude]\n",
		".agnostic-ai/mcps/acme.yaml":   "name: acme-tool\ntype: stdio\ncommand: acme\n",
		".agnostic-ai/mcps/github.yaml": "name: github\ntype: http\nurl: https://api.githubcopilot.com/mcp/\nheaders:\n  Authorization: Bearer ${GITHUB_TOKEN}\n",
	})
	v := buildView(t, root, Options{})

	providers := map[string]Provider{}
	for _, p := range v.Providers {
		providers[p.ID] = p
	}
	acme := providers["mcp-acme-tool"]
	if !slices.Equal(acme.DomainIDs, []string{DomainAgentOps}) || acme.DomainResolution != DomainResolutionDefault {
		t.Errorf("acme = %v %s", acme.DomainIDs, acme.DomainResolution)
	}
	if providers["local-files"].DomainResolution != DomainResolutionBuiltin {
		t.Errorf("local files resolution = %s", providers["local-files"].DomainResolution)
	}
	if len(uncertaintiesWith(v, UncertaintyDomainDefaulted)) != 1 {
		t.Error("no domain-defaulted uncertainty")
	}
	if len(v.Domains) != 8 {
		t.Fatalf("domains = %d, want all eight", len(v.Domains))
	}
	for i, d := range v.Domains {
		if d.Order != i+1 {
			t.Errorf("%s order = %d", d.ID, d.Order)
		}
		if d.ID == DomainSecrets && (!slices.Equal(d.ProviderIDs, []string{"mcp-github"}) || len(d.ResourceIDs) != 0) {
			t.Errorf("secrets domain = providers %v resources %v, want the referencing provider and no resources", d.ProviderIDs, d.ResourceIDs)
		}
		if d.ID == DomainAgentOps && !slices.Contains(d.ResourceIDs, "mcp-acme-tool-scope") {
			t.Errorf("agent ops resources = %v", d.ResourceIDs)
		}
	}
}
