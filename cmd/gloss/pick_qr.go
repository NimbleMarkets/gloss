package main

import (
	"context"
	"fmt"
	"io"
	"net/netip"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/app"
	"github.com/charmbracelet/x/term"
)

// Only a foreground terminal gets an interactive QR. Agent startup remains
// one JSON object, redirected stderr remains text, and loopback is never
// presented as a phone handoff. A hostname's reachability still needs testing.
func pickQRAvailable(opts options, stderr io.Writer) bool {
	output, ok := stderr.(interface{ Fd() uintptr })
	return ok && opts.Detached == "" && term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(output.Fd()) && networkPick(opts)
}

func networkPick(opts options) bool {
	n, err := parsePickNetwork(opts.Listen, opts.AdvertiseHost)
	if err != nil || n.listen.Addr().IsLoopback() || n.host == "localhost" {
		return false
	}
	ip, err := netip.ParseAddr(n.host)
	return err != nil || !ip.IsLoopback()
}

type webPickResult struct {
	paths   []string
	message string
	err     error
}

func waitWebPickQR(ctx context.Context, s *server, opts options, stderr io.Writer) ([]string, string, error) {
	return runWebPickQR(ctx, s, opts, stderr, func(m tea.Model) error {
		_, err := tea.NewProgram(m, tea.WithOutput(stderr), tea.WithoutSignalHandler()).Run()
		return err
	})
}

// Inject only the terminal runner so tests exercise the same server settlement
// and exit-code mapping without owning stdin or a real terminal.
func runWebPickQR(ctx context.Context, s *server, opts options, stderr io.Writer, run func(tea.Model) error) ([]string, string, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	m, err := app.NewHandoff(s.URL, opts.Prompt, opts.Render, done, cancel)
	if err != nil {
		// A long advertised name might exceed QR capacity. The URL and server
		// are still usable; do not discard a request because its QR failed.
		fmt.Fprintf(stderr, "gloss: QR unavailable: %v\n", err)
		return s.waitWebPick(ctx, opts.Timeout)
	}
	result := make(chan webPickResult, 1)
	go func() {
		paths, message, err := s.waitWebPick(ctx, opts.Timeout)
		result <- webPickResult{paths, message, err}
		close(done)
	}()
	// served owns OS signals. Its cancellation settles the server, which lets
	// the screen execute its image cleanup before Bubble Tea restores the TTY.
	screenErr := run(m)
	cancel()
	r := <-result
	if screenErr != nil && r.err != nil {
		return nil, "", fmt.Errorf("handoff screen: %w", screenErr)
	}
	return r.paths, r.message, r.err
}
