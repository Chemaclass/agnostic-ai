package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func IsolateUserCache(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	for key, value := range map[string]string{
		"HOME": base, "USERPROFILE": base,
		"LocalAppData":   filepath.Join(base, "cache"),
		"XDG_CACHE_HOME": filepath.Join(base, "cache"),
	} {
		t.Setenv(key, value)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(base, cache)
	if err != nil || !filepath.IsLocal(rel) {
		t.Fatalf("user cache %s escaped temporary base %s: %v", cache, base, err)
	}
	return base
}
