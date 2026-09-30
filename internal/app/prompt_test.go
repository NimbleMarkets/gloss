package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestPromptIsShownAtTheBottom(t *testing.T) {
	m := viewing(t, Options{Pick: true, Prompt: "Please choose the invoice for March"}, "")
	send(m, tea.WindowSizeMsg{Width: 60, Height: 12})
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	if len(lines) != 12 {
		t.Fatalf("%d lines:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	// The box sits just above the status bar, and the body is shorter by it.
	if !strings.Contains(lines[8], "Please choose the invoice for March") || !strings.Contains(lines[7], "╭") || !strings.Contains(lines[9], "╰") {
		t.Fatalf("no box above the status bar:\n%s", strings.Join(lines, "\n"))
	}
	if m.bodyHeight() != 12-2-3 {
		t.Fatalf("body height %d", m.bodyHeight())
	}
	if !strings.Contains(lines[10], "No files yet") {
		t.Fatalf("status bar moved:\n%s", strings.Join(lines, "\n"))
	}
}

func TestPromptCanBeOnTop(t *testing.T) {
	m := viewing(t, Options{Prompt: "Pick one", PromptTop: true}, "")
	send(m, tea.WindowSizeMsg{Width: 60, Height: 12})
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	if !strings.Contains(lines[0], "╭") || !strings.Contains(lines[1], "Pick one") || !strings.Contains(lines[2], "╰") {
		t.Fatalf("no box on top:\n%s", strings.Join(lines, "\n"))
	}
}

func TestPromptWrapsAndIsCut(t *testing.T) {
	m := viewing(t, Options{Prompt: strings.Repeat("word ", 80) + "\x1b[31mend"}, "")
	send(m, tea.WindowSizeMsg{Width: 40, Height: 20})
	view := ansi.Strip(m.View().Content)
	if strings.Count(view, "word") < 20 || strings.Contains(view, "\x1b") || m.promptHeight() != 6 {
		t.Fatalf("prompt height %d:\n%s", m.promptHeight(), view)
	}
	for _, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) > 40 {
			t.Fatalf("too wide: %q", line)
		}
	}
	// No room, no box.
	send(m, tea.WindowSizeMsg{Width: 40, Height: 6})
	if m.promptHeight() != 0 {
		t.Fatalf("prompt height %d on a short screen", m.promptHeight())
	}
}

func TestPreviewsHaveNoPrompt(t *testing.T) {
	m := viewing(t, Options{Files: []string{samples + "shapes.svg", samples + "readme.md"}, Menu: true, Preview: true, Prompt: "Choose"}, "svg")
	send(m, tea.WindowSizeMsg{Width: 120, Height: 30})
	if m.preview == nil || m.preview.opts.Prompt != "" {
		t.Fatal("the preview carries the prompt")
	}
}
