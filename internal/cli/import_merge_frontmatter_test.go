package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// A target that cannot express a key must not delete it: the spec's own
// argument-hint and allowed-tools survive an import that carries only
// name and description, and the imported description still wins.
func TestMergeSpecFrontmatter_KeepsKeysOnlyTheSpecDeclares(t *testing.T) {
	existing := []byte("---\nname: gh-issues\ndescription: old\nargument-hint: \"[--limit N]\"\nallowed-tools: \"Read, Bash(gh *)\"\n---\n\nold body\n")
	imported := []byte("---\nname: gh-issues\ndescription: new\n---\n\nnew body\n")

	got, err := mergeSpecFrontmatter(existing, imported, defaultSkillFields)
	if err != nil {
		t.Fatalf("mergeSpecFrontmatter: %v", err)
	}
	want := "---\nname: gh-issues\ndescription: new\nargument-hint: \"[--limit N]\"\nallowed-tools: \"Read, Bash(gh *)\"\n---\n\nnew body\n"
	if string(got) != want {
		t.Errorf("merged spec:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// A lossless target keeps today's byte-for-byte write.
func TestMergeSpecFrontmatter_ReturnsImportedVerbatimWhenNothingToCarry(t *testing.T) {
	existing := []byte("---\nname: alpha\ndescription: old\n---\n\nold body\n")
	imported := []byte("---\nname: alpha\ndescription: new\nargument-hint: \"[x]\"\n---\n\nnew body\n")

	got, err := mergeSpecFrontmatter(existing, imported, defaultSkillFields)
	if err != nil {
		t.Fatalf("mergeSpecFrontmatter: %v", err)
	}
	if string(got) != string(imported) {
		t.Errorf("merged spec = %q, want the imported bytes", got)
	}
}

func TestMergeSpecFrontmatter_ExistingWithoutFrontmatterReturnsImported(t *testing.T) {
	imported := []byte("---\nname: alpha\n---\n\nbody\n")

	got, err := mergeSpecFrontmatter([]byte("just a body\n"), imported, defaultSkillFields)
	if err != nil {
		t.Fatalf("mergeSpecFrontmatter: %v", err)
	}
	if string(got) != string(imported) {
		t.Errorf("merged spec = %q, want the imported bytes", got)
	}
}

// A target that flattens frontmatter away leaves the spec's keys in place.
func TestMergeSpecFrontmatter_ImportedWithoutFrontmatterKeepsExistingKeys(t *testing.T) {
	existing := []byte("---\nname: alpha\ntargets: [claude, qoder]\n---\n\nold body\n")

	got, err := mergeSpecFrontmatter(existing, []byte("new body\n"), defaultSkillFields)
	if err != nil {
		t.Fatalf("mergeSpecFrontmatter: %v", err)
	}
	want := "---\nname: alpha\ntargets: [claude, qoder]\n---\n\nnew body\n"
	if string(got) != want {
		t.Errorf("merged spec:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestMergeSpecFrontmatter_AppendsImportedOnlyKeys(t *testing.T) {
	existing := []byte("---\nname: alpha\nallowed-tools: Read\n---\n\nold body\n")
	imported := []byte("---\nname: alpha\npaths: src/**\n---\n\nnew body\n")

	got, err := mergeSpecFrontmatter(existing, imported, defaultSkillFields)
	if err != nil {
		t.Fatalf("mergeSpecFrontmatter: %v", err)
	}
	want := "---\nname: alpha\nallowed-tools: Read\npaths: src/**\n---\n\nnew body\n"
	if string(got) != want {
		t.Errorf("merged spec:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestMergeSpecFrontmatter_MalformedFrontmatterFallsBackToImported(t *testing.T) {
	existing := []byte("---\n- not: a mapping\n---\n\nold body\n")
	imported := []byte("---\nname: alpha\n---\n\nnew body\n")

	got, err := mergeSpecFrontmatter(existing, imported, defaultSkillFields)
	if err != nil {
		t.Fatalf("mergeSpecFrontmatter: %v", err)
	}
	if string(got) != string(imported) {
		t.Errorf("merged spec = %q, want the imported bytes", got)
	}
}

func TestImportWriteSpecMarkdown_WritesPlainWhenSpecMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	data := []byte("---\nname: alpha\n---\n\nbody\n")

	if err := importWriteSpecMarkdown(path, data, 0o644, defaultSkillFields); err != nil {
		t.Fatalf("importWriteSpecMarkdown: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("written spec = %q, want %q", got, data)
	}
}
