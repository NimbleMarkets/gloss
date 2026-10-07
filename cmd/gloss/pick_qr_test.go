package main

import (
	"bytes"
	tea "charm.land/bubbletea/v2"
	"context"
	"errors"
	"github.com/NimbleMarkets/gloss/internal/app"
	"io"
	"os"
	"testing"
	"time"
)

func TestPickQRDoesNotAdvertiseLoopbackAsPhoneHandoff(t *testing.T) {
	for _, tt := range []struct {
		listen, host string
		want         bool
	}{
		{defaultPickListen, "", false},
		{defaultPickListen, "laptop.ts.net", false},
		{"[::1]:0", "", false},
		{"0.0.0.0:0", "127.0.0.1", false},
		{"0.0.0.0:0", "localhost", false},
		{"192.168.9.216:0", "", true},
		{"0.0.0.0:0", "laptop.ts.net", true},
	} {
		opts := options{Listen: tt.listen, AdvertiseHost: tt.host}
		if networkPick(opts) != tt.want {
			t.Errorf("%+v: wrong QR eligibility", tt)
		}
		if pickQRAvailable(opts, &bytes.Buffer{}) {
			t.Fatal("redirected output must remain text")
		}
	}
}

// Run the actual Tea event loop with no TTY or renderer. Server completion must
// close the model, and terminal cancellation must settle the same server wait.
func TestWebPickQRExitCodes(t *testing.T) {
	for _, tt := range []struct {
		action string
		code   int
	}{
		{"send", 0}, {"decline", 2}, {"quit", 2}, {"signal", 2}, {"timeout", 124}, {"screen error", 1},
	} {
		t.Run(tt.action, func(t *testing.T) {
			s := webPicking(t, app.Options{})
			files := pickUpload(t, s, 200, upload{"note.txt", []byte("hello")})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			opts := options{Options: app.Options{Render: "glyph"}, Timeout: 5 * time.Second}
			if tt.action == "timeout" {
				opts.Timeout = 10 * time.Millisecond
			}
			paths, message, err := runWebPickQR(ctx, s, opts, io.Discard, func(m tea.Model) error {
				switch tt.action {
				case "send":
					pickRequest(t, s, "POST", "confirm", confirmMessage("reply", files[0].ID), 200)
				case "decline":
					pickRequest(t, s, "POST", "decline", "", 200)
				case "signal":
					cancel()
				case "screen error":
					return errors.New("screen unavailable")
				}
				screenCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
				defer stop()
				p := tea.NewProgram(m, tea.WithContext(screenCtx), tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler())
				if tt.action == "quit" {
					go p.Send(tea.KeyPressMsg{Code: 'q'})
				}
				_, err := p.Run()
				return err
			})
			if got := exitCode(err); got != tt.code {
				t.Fatalf("exit %d, want %d: %v", got, tt.code, err)
			}
			if tt.action == "send" {
				if len(paths) != 1 || message != "reply" {
					t.Fatalf("lost answer: %v %q", paths, message)
				}
				if data, err := os.ReadFile(paths[0]); err != nil || string(data) != "hello" {
					t.Fatalf("lost upload: %q %v", data, err)
				}
			} else if len(paths) != 0 || message != "" {
				t.Fatalf("unsuccessful answer: %v %q", paths, message)
			}
		})
	}
}
