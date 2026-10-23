package authority

import (
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// domainInfo is one curated domain as the view presents it.
type domainInfo struct {
	id          string
	label       string
	description string
	examples    []string
}

// domainCatalog holds the eight curated domains in display order.
var domainCatalog = []domainInfo{
	{DomainSourceControl, "Source Code & Version Control", "Repositories, branches, pull requests, and issues.", []string{"GitHub", "GitLab", "Bitbucket"}},
	{DomainCloud, "Cloud Infrastructure & Deployment", "Cloud accounts, deployments, and infrastructure.", []string{"Railway", "Vercel", "AWS"}},
	{DomainData, "Data & Storage", "Databases, warehouses, caches, and object stores.", []string{"PostgreSQL", "Snowflake", "Redis"}},
	{DomainFilesystem, "Filesystem & Execution", "Project files and the commands a tool runs on this machine.", []string{"Local files", "Shell commands", "Docker"}},
	{DomainWeb, "Web & Browser", "Fetching pages, searching the web, and driving a browser.", []string{"Web fetch", "Playwright", "Brave Search"}},
	{DomainCommunication, "Communication & Collaboration", "Chat, email, issue trackers, and calendars.", []string{"Slack", "Gmail", "Linear"}},
	{DomainSecrets, "Secrets & Identity", "Credentials the configuration references, and identity providers.", []string{"Vault", "1Password", "Okta"}},
	{DomainAgentOps, "AI & Agent Operations", "Other AI services, and any server the curated table does not place.", []string{"Model APIs", "Agent memory", "Unknown servers"}},
}

// domainKeyword places an MCP server in domains when its name, URL, or
// command holds the keyword. Short or ambiguous keywords match a whole
// word only, so "db" matches analytics-db but not a word that merely
// contains it.
type domainKeyword struct {
	keyword string
	exact   bool
	domains []string
}

var domainKeywords = []domainKeyword{
	{"github", false, []string{DomainSourceControl}},
	{"gitlab", false, []string{DomainSourceControl}},
	{"bitbucket", false, []string{DomainSourceControl}},
	{"aws", true, []string{DomainCloud}},
	{"amazon", false, []string{DomainCloud}},
	{"gcp", true, []string{DomainCloud}},
	{"google-cloud", false, []string{DomainCloud}},
	{"azure", false, []string{DomainCloud}},
	{"railway", false, []string{DomainCloud}},
	{"vercel", false, []string{DomainCloud}},
	{"netlify", false, []string{DomainCloud}},
	{"fly", true, []string{DomainCloud}},
	{"heroku", false, []string{DomainCloud}},
	{"render", true, []string{DomainCloud}},
	{"kubernetes", false, []string{DomainCloud}},
	{"terraform", false, []string{DomainCloud}},
	{"cloudflare", false, []string{DomainCloud}},
	{"postgres", false, []string{DomainData}},
	{"mysql", false, []string{DomainData}},
	{"sqlite", false, []string{DomainData}},
	{"mongo", false, []string{DomainData}},
	{"redis", false, []string{DomainData}},
	{"snowflake", false, []string{DomainData}},
	{"bigquery", false, []string{DomainData}},
	{"supabase", false, []string{DomainData}},
	{"db", true, []string{DomainData}},
	{"database", false, []string{DomainData}},
	{"sql", true, []string{DomainData}},
	{"s3", true, []string{DomainCloud, DomainData}},
	{"filesystem", false, []string{DomainFilesystem}},
	{"fs", true, []string{DomainFilesystem}},
	{"sandbox", false, []string{DomainFilesystem}},
	{"docker", false, []string{DomainFilesystem}},
	{"browser", false, []string{DomainWeb}},
	{"playwright", false, []string{DomainWeb}},
	{"puppeteer", false, []string{DomainWeb}},
	{"fetch", false, []string{DomainWeb}},
	{"http", true, []string{DomainWeb}},
	{"search", true, []string{DomainWeb}},
	{"brave", false, []string{DomainWeb}},
	{"slack", false, []string{DomainCommunication}},
	{"gmail", false, []string{DomainCommunication}},
	{"email", false, []string{DomainCommunication}},
	{"linear", true, []string{DomainCommunication}},
	{"jira", true, []string{DomainCommunication}},
	{"notion", false, []string{DomainCommunication}},
	{"discord", false, []string{DomainCommunication}},
	{"calendar", false, []string{DomainCommunication}},
	{"confluence", false, []string{DomainCommunication}},
	{"vault", false, []string{DomainSecrets}},
	{"1password", false, []string{DomainSecrets}},
	{"secrets", false, []string{DomainSecrets}},
	{"iam", true, []string{DomainSecrets}},
	{"okta", true, []string{DomainSecrets}},
	{"auth0", false, []string{DomainSecrets}},
	{"doppler", false, []string{DomainSecrets}},
}

// mcpDomains places an MCP server through the curated keyword table, in
// catalog order. It returns nil when no keyword matches.
func mcpDomains(e spec.Entry) []string {
	words := domainWords(e)
	joined := "-" + strings.Join(words, "-") + "-"
	found := map[string]bool{}
	for _, k := range domainKeywords {
		if keywordMatches(k, words, joined) {
			for _, d := range k.domains {
				found[d] = true
			}
		}
	}
	var out []string
	for _, d := range domainCatalog {
		if found[d.id] {
			out = append(out, d.id)
		}
	}
	return out
}

func keywordMatches(k domainKeyword, words []string, joined string) bool {
	if strings.Contains(k.keyword, "-") {
		return strings.Contains(joined, "-"+k.keyword+"-")
	}
	for _, w := range words {
		if w == k.keyword || !k.exact && strings.Contains(w, k.keyword) {
			return true
		}
	}
	return false
}

// domainWords splits the server's name, URL without its scheme,
// command, and package arguments into lowercase words of letters and
// digits. A stdio server launched through npx, uvx, or docker is named
// by its package argument, such as @modelcontextprotocol/server-postgres,
// so that argument counts like the command.
func domainWords(e spec.Entry) []string {
	url, _ := e.Meta["url"].(string)
	if _, rest, ok := strings.Cut(url, "://"); ok {
		url = rest
	}
	command, _ := e.Meta["command"].(string)
	texts := []string{e.Name, url, command}
	args, _ := e.Meta["args"].([]any)
	for _, arg := range args {
		if s, ok := arg.(string); ok && packageArgument(s) {
			texts = append(texts, s)
		}
	}
	var words []string
	for _, text := range texts {
		words = append(words, strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
			return (r < 'a' || r > 'z') && (r < '0' || r > '9')
		})...)
	}
	return words
}

// packageArgument reports an argument that names the server package or
// image, never a flag, a path, or a reference.
func packageArgument(arg string) bool {
	if arg == "" || strings.ContainsAny(arg[:1], "-/.~") || strings.Contains(arg, "${") {
		return false
	}
	lower := strings.ToLower(arg)
	return strings.HasPrefix(lower, "@") || strings.Contains(lower, "mcp") || strings.Contains(lower, "server")
}

// assembleDomains fills each curated domain with its providers and
// resources and counts the resources' operations by effective effect.
// The secrets domain lists every provider that references a secret and
// holds no resources: a client renders View.Secrets there.
func assembleDomains(providers []Provider, resources []Resource) []Domain {
	out := make([]Domain, 0, len(domainCatalog))
	for i, info := range domainCatalog {
		d := Domain{
			ID:          info.id,
			Label:       info.label,
			Description: info.description,
			Examples:    slices.Clone(info.examples),
			Order:       i + 1,
			ProviderIDs: []string{},
			ResourceIDs: []string{},
		}
		for _, p := range providers {
			if slices.Contains(p.DomainIDs, info.id) || info.id == DomainSecrets && len(p.SecretIDs) > 0 {
				d.ProviderIDs = append(d.ProviderIDs, p.ID)
			}
		}
		for _, r := range resources {
			if !slices.Contains(r.DomainIDs, info.id) {
				continue
			}
			d.ResourceIDs = append(d.ResourceIDs, r.ID)
			for _, op := range r.Operations {
				countEffect(&d.Summary, op.Effect)
			}
		}
		out = append(out, d)
	}
	return out
}

func countEffect(c *EffectCounts, e Effect) {
	switch e {
	case EffectAllowed:
		c.Allowed++
	case EffectAsk:
		c.Ask++
	case EffectDenied:
		c.Denied++
	case EffectUnavailable:
		c.Unavailable++
	case EffectDefault:
		c.Default++
	case EffectUnresolved:
		c.Unresolved++
	}
}

// resourceDomains is a provider's domains without the secrets domain,
// whose resources a client takes from View.Secrets instead.
func resourceDomains(providerDomains []string) []string {
	out := []string{}
	for _, d := range providerDomains {
		if d != DomainSecrets {
			out = append(out, d)
		}
	}
	return out
}
