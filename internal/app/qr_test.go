//go:build !js

package app

import (
	"bytes"
	"image/png"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/NimbleMarkets/ntcharts/v2/picture"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/makiuchi-d/gozxing"
	decoder "github.com/makiuchi-d/gozxing/qrcode"
)

func qrTable(t *testing.T) *Model {
	t.Helper()
	return tabulated(t, &document.Sheet{Name: "Links", Columns: 1, Rows: [][]string{{DocsURL}, {"https://example.org/second"}}}, 1, 1)
}

func TestTableQRIsExplicitAndPreservesTheURL(t *testing.T) {
	m := qrTable(t)
	if m.qr != nil || !strings.Contains(m.hints(), "u QR") || m.opts.Fetch != nil {
		t.Fatal("QR must be explicit and independent of --fetch")
	}
	m.Update(press("u"))
	if m.layer != layerQR || m.qr == nil || m.qr.address != DocsURL {
		t.Fatal("u did not open the selected URL")
	}
	for _, line := range strings.Split(m.qr.model.View(), "\n") {
		if !strings.Contains(m.View().Content, line) {
			t.Fatal("overlay cropped a QR row")
		}
	}
	if !strings.Contains(ansi.Strip(m.View().Content), DocsURL) {
		t.Fatal("original text missing")
	}
	m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	m.Update(press("down"))
	if m.sheet.url() != DocsURL {
		t.Fatal("overlay let navigation change the URL behind the code")
	}
	m.Update(press("?"))
	if strings.Contains(m.View().Content, "▀") {
		t.Fatal("QR covered the help")
	}
	m.Update(press("?"))
	m.Update(tea.WindowSizeMsg{Width: 30, Height: 15})
	if strings.Contains(m.qr.model.View(), "▀") || m.qr.model.Err() == nil {
		t.Fatal("small window cropped QR instead of reporting fit")
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	if m.qr.model.Err() != nil {
		t.Fatal("enlarging did not recover")
	}
	// A measured, slightly non-square half-cell must not require hundreds
	// of columns. Exercise the geometry reply through the host event loop.
	m.Update(uv.CellSizeEvent{Width: 8, Height: 17})
	if m.qr.model.Err() != nil || !strings.Contains(m.View().Content, "▀") {
		t.Fatalf("ordinary cell proportions lost the QR: %v", m.qr.model.Err())
	}
	m.Update(press("esc"))
	if m.qr != nil || m.layer != layerNone || m.sheet.url() != DocsURL {
		t.Fatal("close lost the table or retained QR")
	}
	m.Update(press("down"))
	m.Update(press("u"))
	if m.qr.address != "https://example.org/second" {
		t.Fatal("next selection opened old QR")
	}
}

func TestTableQRKittyLifecycle(t *testing.T) {
	t.Cleanup(func() { picture.ForceKittyCapability(picture.KittyCapabilityUnknown) })
	for _, action := range []string{"esc", "q", "Q", "replace", "load", "browser", "resize", "fallback"} {
		t.Run(action, func(t *testing.T) {
			m := qrTable(t)
			picture.ForceKittyCapability(picture.KittyCapabilitySupported)
			m.pic.Toggle()
			_, cmd := m.Update(press("u"))
			pump(m, cmd, 0)
			if !strings.ContainsRune(m.View().Content, kitty.Placeholder) {
				t.Fatal("QR frames were not routed through the app event loop")
			}
			_, cmd = m.Update(uv.CellSizeEvent{Width: 10, Height: 20})
			pump(m, cmd, 0)
			if !strings.ContainsRune(m.View().Content, kitty.Placeholder) {
				t.Fatal("cell geometry update lost QR")
			}
			old := m.qr.model
			switch action {
			case "replace":
				cmd = m.openQR("https://example.org/replaced")
			case "load":
				_, cmd = m.Update(document.Result{Generation: m.generation, Kind: "text"})
			case "browser":
				cmd = m.openBrowser()
			case "resize":
				_, cmd = m.Update(tea.WindowSizeMsg{Width: 5, Height: 5})
			case "fallback":
				_, cmd = m.Update(press("g"))
			default:
				_, cmd = m.Update(press(action))
			}
			raw := collectRaw(cmd)
			if !strings.Contains(raw, "a=d,d=I,i=") {
				t.Fatalf("%s omitted ID-specific cleanup: %q", action, raw)
			}
			if action != "resize" && action != "fallback" && old.View() != "" {
				t.Fatalf("%s left removed component alive", action)
			}
		})
	}
}

func TestTableQRExportUsesExistingSaveHook(t *testing.T) {
	for _, address := range []string{
		DocsURL,
		"https://example.org/東京?q=☕&sku=12345678901234567890",
		"https://example.org/東京/〜−‖¢£¬",
	} {
		t.Run(address, func(t *testing.T) {
			m := tabulated(t, &document.Sheet{Name: "Links", Columns: 1, Rows: [][]string{{address}}}, 1, 1)
			m.Update(press("u"))
			var saved []byte
			m.opts.Save = func(name string, data []byte) (string, error) {
				if name != "qr.png" {
					t.Fatalf("name %s", name)
				}
				saved = bytes.Clone(data)
				return name, nil
			}
			_, cmd := m.Update(press("e"))
			pump(m, cmd, 0)
			img, err := png.Decode(bytes.NewReader(saved))
			if err != nil {
				t.Fatal(err)
			}
			bitmap, _ := gozxing.NewBinaryBitmapFromImage(img)
			result, err := decoder.NewQRCodeReader().Decode(bitmap, nil)
			if err != nil || result.GetText() != address || !strings.Contains(m.note, "saved qr.png") {
				t.Fatalf("export did not retain exact URL: %v, %s", err, m.note)
			}
		})
	}
}
