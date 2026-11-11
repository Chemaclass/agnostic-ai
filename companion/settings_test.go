package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSettings_AnimateIsOnWhenTheFileHasNoKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "companion.json")
	if err := os.WriteFile(path, []byte(`{"shortcut":"Cmd+Shift+K"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	saved, err := readSettings(path)
	if err != nil {
		t.Fatal(err)
	}

	if !saved.animate() {
		t.Error("a file without the key turned animation off")
	}
}

func TestSettings_KeepsAnimateOffAcrossAWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "companion.json")
	off := false
	written := settings{Bubble: &point{X: 10, Y: 20}, Shortcut: "Cmd+Shift+K", Animate: &off}

	if err := writeSettings(path, written); err != nil {
		t.Fatal(err)
	}
	read, err := readSettings(path)
	if err != nil {
		t.Fatal(err)
	}

	if read.animate() {
		t.Error("animation came back on")
	}
	if read.Bubble == nil || *read.Bubble != (point{X: 10, Y: 20}) || read.Shortcut != "Cmd+Shift+K" {
		t.Errorf("the other settings changed: %+v", read)
	}
}

func TestSettings_DimIsOffWhenTheFileHasNoKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "companion.json")
	if err := os.WriteFile(path, []byte(`{"shortcut":"Cmd+Shift+K"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	saved, err := readSettings(path)
	if err != nil {
		t.Fatal(err)
	}

	if saved.DimWhenIdle {
		t.Error("a file without the key turned dimming on")
	}
}

func TestSettings_KeepsDimOnAcrossAWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "companion.json")
	written := settings{Bubble: &point{X: 10, Y: 20}, Shortcut: "Cmd+Shift+K", DimWhenIdle: true}

	if err := writeSettings(path, written); err != nil {
		t.Fatal(err)
	}
	read, err := readSettings(path)
	if err != nil {
		t.Fatal(err)
	}

	if !read.DimWhenIdle {
		t.Error("dimming came back off")
	}
	if read.Bubble == nil || *read.Bubble != (point{X: 10, Y: 20}) || read.Shortcut != "Cmd+Shift+K" || !read.animate() {
		t.Errorf("the other settings changed: %+v", read)
	}
}
