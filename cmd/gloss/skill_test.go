package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NimbleMarkets/gloss/skills"
)

func TestSkillInstall(t *testing.T) {
	// --install needs --skill.
	if _, _, err := parse([]string{"--install"}, &bytes.Buffer{}); err == nil {
		t.Fatal("--install alone was accepted")
	}
	// A named skills folder gets gloss/SKILL.md, whole; the path is the answer.
	dir := t.TempDir()
	root := filepath.Join(dir, "skills")
	opts, done, err := parse([]string{"--skill", "--install=" + root}, &bytes.Buffer{})
	if err != nil || done || opts.SkillInstall != root {
		t.Fatalf("parse = %+v, done=%v, %v", opts, done, err)
	}
	var stdout, stderr bytes.Buffer
	if err := installSkill(opts.SkillInstall, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	written := filepath.Join(root, "gloss", "SKILL.md")
	if strings.TrimSpace(stdout.String()) != written {
		t.Fatalf("stdout = %q, want %q", stdout.String(), written)
	}
	content, err := os.ReadFile(written)
	if err != nil || string(content) != skills.Gloss {
		t.Fatalf("%s: %v", written, err)
	}
	// Again, and it is an update, said as such.
	stderr.Reset()
	if err := installSkill(root, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "updated") {
		t.Fatalf("reinstall not called an update: %q", stderr.String())
	}
}

func TestSkillInstallAuto(t *testing.T) {
	// Detection looks in the home folder; give it one, with Claude Code in it.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KIMI_CODE_HOME", "")
	if err := os.Mkdir(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := installSkill("auto", &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".claude", "skills", "gloss", "SKILL.md")
	if strings.TrimSpace(stdout.String()) != want || !strings.Contains(stderr.String(), "Claude Code") {
		t.Fatalf("stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatal(err)
	}
	// Kimi Code's home, moved by KIMI_CODE_HOME, is found as well.
	kimi := filepath.Join(home, "kimi-home")
	t.Setenv("KIMI_CODE_HOME", kimi)
	if err := os.Mkdir(kimi, 0o755); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := installSkill("auto", &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), filepath.Join(kimi, "skills", "gloss", "SKILL.md")) {
		t.Fatalf("Kimi Code's folder missed: %q", stdout.String())
	}
}

func TestSkillInstallFindsNoAgent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KIMI_CODE_HOME", "")
	var stdout bytes.Buffer
	if err := installSkill("auto", &stdout, &bytes.Buffer{}); err == nil || stdout.Len() != 0 {
		t.Fatalf("err = %v, stdout = %q", err, stdout.String())
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	if expandHome("~/skills") != filepath.Join(home, "skills") || expandHome("~") != home || expandHome("/tmp/x") != "/tmp/x" {
		t.Fatalf("expandHome: ~=%q ~/skills=%q", expandHome("~"), expandHome("~/skills"))
	}
}
