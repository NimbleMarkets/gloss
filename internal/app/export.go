package app

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
)

type exportResult struct {
	name string
	size image.Point
	err  error
}

func (r exportResult) note() string {
	if r.err != nil {
		return "export failed: " + safe(r.err.Error())
	}
	return fmt.Sprintf("saved %s · %d×%d", safe(r.name), r.size.X, r.size.Y)
}

// exportRequest asks the loader for the page as it is on screen: the same
// parts and paint, at export size.
func (m *Model) exportRequest() document.Request {
	q := m.request(false)
	q.Preview, q.MaxEdge = false, m.opts.MaxEdge
	if q.MaxEdge == 0 {
		q.MaxEdge = 1536
	}
	return q
}

// export renders the current page as --output would, from the source rather
// than the terminal's cells. A mesh keeps the camera it is being viewed from.
func (m *Model) export() tea.Cmd {
	if m.loading || m.err != nil || m.kind == "" {
		return nil
	}
	if m.markdown != nil {
		m.note = "Markdown cannot be exported as PNG"
		return nil
	}
	if m.sheet != nil {
		m.note = "spreadsheets cannot be exported as PNG"
		return nil
	}
	q := m.exportRequest()
	edge := q.MaxEdge
	name, profile, save := exportName(q.Path, m.kind, m.page), m.opts.VisionProfile, m.opts.Save
	if save == nil {
		save = func(name string, data []byte) (string, error) { return saveFile(".", name, data) }
	}
	view := document.Result{CPU: m.exportOnCPU()}
	if m.chart != nil {
		camera := m.chart.Camera()
		view.Camera = &camera
	}
	m.note = "exporting…"
	return func() tea.Msg {
		r := m.loader.Load(q)
		r.Camera, r.CPU = view.Camera, view.CPU
		img, err := document.ExportForVision(r, edge, profile)
		if err != nil {
			return exportResult{err: err}
		}
		var data bytes.Buffer
		if err := png.Encode(&data, img); err != nil {
			return exportResult{err: err}
		}
		name, err := save(name, data.Bytes())
		return exportResult{name: name, size: img.Bounds().Size(), err: err}
	}
}

// exportOnCPU reports whether a renderer other than the GPU was asked for.
func (m *Model) exportOnCPU() bool { return m.opts.Render3D != "auto" && m.opts.Render3D != "" }

func exportName(path, kind string, page int) string {
	base := filepath.Base(path)
	if ext := filepath.Ext(base); ext != base {
		base = strings.TrimSuffix(base, ext)
	}
	if document.IsStdin(path) {
		base = "stdin"
	}
	if kind == "pdf" {
		base += fmt.Sprintf("-page-%d", page)
	}
	return base + ".png"
}

// saveFile never replaces a file: a taken name gains a number instead.
func saveFile(dir, name string, data []byte) (string, error) {
	stem := strings.TrimSuffix(name, ".png")
	for n := 1; n < 1000; n++ {
		if n > 1 {
			name = fmt.Sprintf("%s-%d.png", stem, n)
		}
		f, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, writeErr := f.Write(data)
		if err := errors.Join(writeErr, f.Close()); err != nil {
			_ = os.Remove(f.Name())
			return "", err
		}
		return name, nil
	}
	return "", fmt.Errorf("%s: too many exports with this name", name)
}
