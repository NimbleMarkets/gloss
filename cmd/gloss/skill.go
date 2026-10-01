package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/NimbleMarkets/gloss/skills"
)

// skillHome is a skills folder an agent reads, detected by the presence of
// detect. For the shared folder, that is the folder itself: it has no agent's
// home to give it away.
type skillHome struct {
	agent, detect, root string
}

// skillHomes are the user-level skills folders gloss knows, in the order they
// are reported.
func skillHomes() []skillHome {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	kimi := os.Getenv("KIMI_CODE_HOME")
	if kimi == "" {
		kimi = filepath.Join(home, ".kimi-code")
	}
	shared := filepath.Join(home, ".agents", "skills")
	return []skillHome{
		{"Claude Code", filepath.Join(home, ".claude"), filepath.Join(home, ".claude", "skills")},
		{"Kimi Code", kimi, filepath.Join(kimi, "skills")},
		{"the shared agents folder (Codex, Cursor, Gemini, Copilot, Kimi)", shared, shared},
	}
}

// installSkill writes the embedded skill as gloss/SKILL.md under each skills
// folder found on this machine, or under the one named. The paths written go
// to stdout, one to a line; who each is for goes to stderr. A skill already
// there is updated: it is meant to match the binary.
func installSkill(where string, stdout, stderr io.Writer) error {
	var homes []skillHome
	if where == "auto" {
		for _, h := range skillHomes() {
			if _, err := os.Stat(h.detect); err == nil {
				homes = append(homes, h)
			}
		}
		if len(homes) == 0 {
			return errors.New("found no agent's skills folder; name one: gloss --skill --install=$HOME/.claude/skills")
		}
	} else {
		homes = []skillHome{{root: expandHome(where)}}
	}
	for _, h := range homes {
		dir := filepath.Join(h.root, "gloss")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		path := filepath.Join(dir, "SKILL.md")
		verb := "installed"
		if _, err := os.Stat(path); err == nil {
			verb = "updated"
		}
		if err := os.WriteFile(path, []byte(skills.Gloss), 0o644); err != nil {
			return err
		}
		if h.agent != "" {
			fmt.Fprintf(stderr, "gloss: skill %s for %s: %s\n", verb, h.agent, path)
		} else {
			fmt.Fprintf(stderr, "gloss: skill %s: %s\n", verb, path)
		}
		fmt.Fprintln(stdout, path)
	}
	return nil
}

// expandHome resolves a leading ~/ , which the shell leaves alone after =.
func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}
