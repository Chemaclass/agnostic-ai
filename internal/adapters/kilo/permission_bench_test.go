package kilo

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// BenchmarkAgentPermission measures one agentPermission call, the
// pairwise rule merge every Kilo agent pays through kiloDefaults.
//
//	status-quo:    every comparison compiles its pattern (the code before the cache)
//	proposed:      compiled matchers cached per pattern, warm
//	proposed-cold: the same cache, emptied before every call
//	baseline:      a direct wildcard matcher with no regexp
func BenchmarkAgentPermission(b *testing.B) {
	narrow := []string{"Read", "Grep", "Bash"}
	wide := []string{
		"Read", "Grep", "Glob", "Edit(src/**)", "Write(docs/*.md)",
		"Bash(git:*)", "Bash(npm run test)", "Bash(go test:*)", "Bash(make lint)",
		"WebFetch(https://example.test/*)",
		"mcp__github__*", "mcp__github__create_issue", "mcp__slack__post_message", "mcp__db__query",
	}
	servers := []string{"github", "github_enterprise", "slack", "db"}
	cases := []struct {
		name    string
		tools   []string
		project []flatRule
	}{
		{"narrow/rules=0", narrow, nil},
		{"narrow/rules=20", narrow, benchProjectRules()},
		{"wide/rules=0", wide, nil},
		{"wide/rules=20", wide, benchProjectRules()},
	}
	subjects := []struct {
		name  string
		cache func() *wildcardCache
		cold  bool
	}{
		{"status-quo", func() *wildcardCache { return &wildcardCache{compile: compileWildcard} }, false},
		{"proposed", func() *wildcardCache { return &wildcardCache{compile: compileWildcard, limit: wildcardCacheLimit} }, false},
		{"proposed-cold", func() *wildcardCache { return &wildcardCache{compile: compileWildcard, limit: wildcardCacheLimit} }, true},
		{"baseline", func() *wildcardCache { return &wildcardCache{compile: directWildcard} }, false},
	}
	original := wildcardPatterns
	b.Cleanup(func() { wildcardPatterns = original })
	for _, c := range cases {
		wildcardPatterns = &wildcardCache{compile: compileWildcard}
		want, _ := agentPermission(c.tools, c.project, servers)
		for _, subject := range subjects {
			wildcardPatterns = subject.cache()
			if got, _ := agentPermission(c.tools, c.project, servers); !reflect.DeepEqual(got, want) {
				b.Fatalf("%s/%s: permission differs from status-quo", c.name, subject.name)
			}
			b.Run(c.name+"/"+subject.name, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if subject.cold {
						wildcardPatterns = subject.cache()
					}
					agentPermission(c.tools, c.project, servers)
				}
			})
		}
	}
}

func benchProjectRules() []flatRule {
	rules := []flatRule{
		{"bash", "rm *", "deny"},
		{"bash", "git push *", "ask"},
		{"bash", "curl *", "ask"},
		{"read", "secrets/*", "deny"},
		{"read", "*.pem", "deny"},
		{"edit", "*.lock", "ask"},
		{"edit", "vendor/**", "deny"},
		{"write", "*.env", "deny"},
		{"webfetch", "*", "ask"},
		{"github_*", "*", "ask"},
		{"github_delete_repo", "*", "deny"},
		{"slack_*", "*", "ask"},
	}
	for i := len(rules); i < 20; i++ {
		rules = append(rules, flatRule{"bash", fmt.Sprintf("tool%d *", i), "allow"})
	}
	return rules
}

// directWildcard matches like compileWildcard without building a
// regular expression.
func directWildcard(pattern string) func(string) bool {
	body, optionalTail := strings.CutSuffix(pattern, " *")
	glob := []rune(body)
	return func(input string) bool {
		text := []rune(input)
		if globMatch(glob, text) {
			return true
		}
		if !optionalTail {
			return false
		}
		for i := range text {
			if text[i] == ' ' && globMatch(glob, text[:i]) {
				return true
			}
		}
		return false
	}
}

func globMatch(glob, text []rune) bool {
	g, t, star, mark := 0, 0, -1, 0
	for t < len(text) {
		switch {
		case g < len(glob) && (glob[g] == '?' || glob[g] == text[t]):
			g++
			t++
		case g < len(glob) && glob[g] == '*':
			star, mark = g, t
			g++
		case star >= 0:
			mark++
			g, t = star+1, mark
		default:
			return false
		}
	}
	for g < len(glob) && glob[g] == '*' {
		g++
	}
	return g == len(glob)
}

func TestWildcardCacheEmptiesAtLimit(t *testing.T) {
	cache := &wildcardCache{compile: compileWildcard, limit: 2}
	for _, pattern := range []string{"a*", "b*", "c*"} {
		if match := cache.matcher(pattern); match == nil || !match(pattern[:1]+"x") {
			t.Fatalf("pattern %q does not match", pattern)
		}
	}
	if len(cache.matchers) != 1 {
		t.Fatalf("cache holds %d patterns, want 1 after emptying at the limit", len(cache.matchers))
	}
}

func TestWildcardCacheConcurrentUse(t *testing.T) {
	cache := &wildcardCache{compile: compileWildcard, limit: 8}
	done := make(chan bool)
	for g := range 8 {
		go func() {
			for i := range 200 {
				pattern := fmt.Sprintf("cmd%d *", (g+i)%20)
				if !cache.matcher(pattern)(strings.TrimSuffix(pattern, " *")) {
					done <- false
					return
				}
			}
			done <- true
		}()
	}
	for range 8 {
		if !<-done {
			t.Fatal("concurrent match failed")
		}
	}
}
