package app

import (
	"bytes"
	"image/png"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/ntcharts-qrcode/qrcode"
	"github.com/charmbracelet/x/ansi"
)

type qrOverlay struct {
	model   *qrcode.Model
	code    *qrcode.Code
	address string
}

func (m *Model) openQR(address string) tea.Cmd {
	code, err := qrcode.Encode(address, qrcode.Options{})
	if err != nil {
		m.note = "QR: " + err.Error()
		return nil
	}
	model, err := qrcode.New(code, qrcode.Config{NextID: nextKittyID})
	if err != nil {
		m.note = "QR: " + err.Error()
		return nil
	}
	cleanup := m.closeLayer()
	m.qr = &qrOverlay{model: model, code: code, address: address}
	m.layer = layerQR
	return tea.Sequence(cleanup, m.layoutQR())
}

func (m *Model) layoutQR() tea.Cmd {
	if m.qr == nil {
		return nil
	}
	// Only the border and a single URL footer surround the QR. Its own quiet
	// zone supplies the whitespace; the code is never truncated or cropped.
	return m.qr.model.SetSize(max(0, m.width-2), max(0, m.bodyHeight()-3))
}

func (m *Model) clearQR() tea.Cmd {
	if m.qr == nil {
		return nil
	}
	cmd := m.qr.model.Close()
	m.qr = nil
	if m.layer == layerQR {
		m.layer = layerNone
	}
	return cmd
}

func (m *Model) qrView(w, h int) string {
	if m.qr == nil || w < 8 || h < 4 {
		return "" // The status hint still offers Esc and export in tiny windows.
	}
	inner := w - 2
	var lines []string
	if err := m.qr.model.Err(); err != nil {
		lines = append(lines, boxText.Render(ansi.Truncate(err.Error(), inner, "…")))
	} else {
		lines = append(lines, strings.Split(m.qr.model.View(), "\n")...)
	}
	// Keep the source visible without letting a long URL add rows. Closing
	// the overlay returns to the original table cell with the complete text.
	lines = append(lines, boxDim.Render(ansi.Truncate(safe(m.qr.address), inner, "…")))
	return boxWithPadding(lines, 0)
}

func (m *Model) exportQR() tea.Cmd {
	if m.qr == nil {
		return nil
	}
	code, save := m.qr.code, m.opts.Save
	if save == nil {
		save = func(name string, data []byte) (string, error) { return saveFile(".", name, data) }
	}
	m.note = "exporting QR…"
	return func() tea.Msg {
		img, err := code.Image(8)
		if err != nil {
			return exportResult{err: err}
		}
		var data bytes.Buffer
		if err := png.Encode(&data, img); err != nil {
			return exportResult{err: err}
		}
		name, err := save("qr.png", data.Bytes())
		return exportResult{name: name, size: img.Bounds().Size(), err: err}
	}
}
