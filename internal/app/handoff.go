package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/ntcharts-qrcode/qrcode"
	"github.com/NimbleMarkets/ntcharts/v2/picture"
	"github.com/charmbracelet/x/ansi"
)

// Handoff displays the already-running web picker's URL. The CLI owns the
// server, its lifetime, and results; this screen owns only terminal graphics.
type Handoff struct {
	pic             picture.Model
	qr              *qrcode.Model
	address, prompt string
	width, height   int
	autoKitty       bool
	closing         bool
	finished        bool
	done            <-chan struct{}
	cancel          func()
}

type handoffFinished struct{}

func NewHandoff(address, prompt, render string, done <-chan struct{}, cancel func()) (*Handoff, error) {
	code, err := qrcode.Encode(address, qrcode.Options{})
	if err != nil {
		return nil, err
	}
	pic, _ := terminalPicture(render, true)
	qr, err := qrcode.New(code, qrcode.Config{NextID: nextKittyID})
	if err != nil {
		return nil, err
	}
	return &Handoff{pic: pic, qr: qr, address: address, prompt: prompt, autoKitty: render != "glyph", done: done, cancel: cancel}, nil
}

func (m *Handoff) Init() tea.Cmd {
	return tea.Batch(m.pic.Init(), func() tea.Msg {
		<-m.done
		return handoffFinished{}
	})
}

func (m *Handoff) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.finished {
		return m, nil
	}
	var commands []tea.Cmd
	switch v := msg.(type) {
	case handoffFinished:
		m.finished = true
		return m, tea.Sequence(m.qr.Close(), tea.Quit)
	case tea.WindowSizeMsg:
		m.width, m.height = max(0, v.Width), max(0, v.Height)
		commands = append(commands, m.pic.SetSize(m.width, m.height))
	case tea.KeyPressMsg:
		switch v.String() {
		case "q", "esc", "ctrl+c":
			if !m.closing {
				m.closing = true
				return m, func() tea.Msg { m.cancel(); return nil }
			}
		case "g":
			m.autoKitty = false
			commands = append(commands, m.pic.Toggle())
		}
	}
	commands = append(commands, updateTerminalPicture(&m.pic, m.autoKitty, msg))
	cw, ch := m.pic.CellPixelSize()
	commands = append(commands, m.qr.SetTerminal(m.pic.Mode() == picture.PictureKitty, cw, ch), m.qr.SetSize(m.width, max(0, m.height-m.textRows())), m.qr.Update(msg))
	return m, tea.Batch(commands...)
}

func (m *Handoff) textRows() int {
	if m.prompt != "" {
		return 4
	}
	return 3
}

func (m *Handoff) View() tea.View {
	var lines []string
	line := func(text string) string { return ansi.Truncate(safe(text), max(1, m.width), "…") }
	if m.height <= m.textRows() {
		lines = []string{line("Enlarge terminal to show the QR")}
	} else {
		lines = append(lines, line("Scan to send files and a message"))
		if m.prompt != "" {
			lines = append(lines, line(strings.Join(strings.Fields(m.prompt), " ")))
		}
		if err := m.qr.Err(); err != nil {
			lines = append(lines, line(err.Error()+"; enlarge terminal"))
		} else {
			// Never truncate or reflow the code or its quiet zone.
			lines = append(lines, m.qr.View())
		}
		lines = append(lines, line(m.address))
		if m.closing {
			lines = append(lines, line("Closing request…"))
		} else {
			lines = append(lines, line("Waiting for Send on your device · g graphics · q/Esc cancel"))
		}
	}
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	return v
}
