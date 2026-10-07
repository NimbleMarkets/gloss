package app

import (
	"bytes"
	"context"
	"fmt"
	"github.com/NimbleMarkets/gloss/examples"
	"image"
	"image/color"
	"image/gif"
	"io"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/ntcharts/v2/picture"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

func animated(t *testing.T, render string, loop int) *Model {
	t.Helper()
	palette := color.Palette{color.RGBA{255, 0, 0, 255}, color.RGBA{0, 255, 0, 255}}
	first, second := image.NewPaletted(image.Rect(0, 0, 4, 2), palette), image.NewPaletted(image.Rect(0, 0, 4, 2), palette)
	for i := range second.Pix {
		second.Pix[i] = 1
	}
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{65535, 2}, LoopCount: loop}); err != nil {
		t.Fatal(err)
	}
	m := New(Options{Files: []string{"motion.gif", "still.gif"}, FilesFS: fstest.MapFS{"motion.gif": {Data: b.Bytes()}, "still.gif": {Data: b.Bytes()}}, Render: render, Page: 1})
	t.Cleanup(func() { m.Close(); picture.ForceKittyCapability(picture.KittyCapabilityUnknown) })
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 12})
	m.generation = 1
	cmd := m.loaded(m.loader.Load(m.request(false)))
	if m.animation == nil {
		t.Fatalf("animation missing: %v", m.err)
	}
	// Drain only rendering commands: tests drive time explicitly below.
	m.animation.paused = true
	pump(m, cmd, 0)
	m.animation.paused = false
	return m
}

func stepAnimation(t *testing.T, m *Model) tea.Cmd {
	t.Helper()
	a := m.animation
	cmd := m.animationTick(animationTick{a, a.epoch})
	if cmd == nil {
		t.Fatal("no compositor command")
	}
	msg, ok := cmd().(animationFrame)
	if !ok {
		t.Fatal("no composed frame")
	}
	return m.animationFrame(msg)
}

func TestGIFPlaybackPauseAndFiniteLoops(t *testing.T) {
	m := animated(t, "glyph", -1)
	a := m.animation
	timer := m.scheduleAnimation()
	if timer == nil || !a.pending || m.scheduleAnimation() != nil {
		t.Fatal("missing or duplicate playback timer")
	}
	old := animationTick{a, a.epoch}
	m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	if !a.paused || a.pending {
		t.Fatal("Space did not pause")
	}
	if timer() != nil {
		t.Fatal("paused timer was not cancelled")
	}
	if cmd := m.animationTick(old); cmd != nil {
		t.Fatal("stale tick advanced paused GIF")
	}
	m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	if a.paused || !a.pending {
		t.Fatal("Space did not resume")
	}
	stepAnimation(t, m)
	if a.player.Frame() != 2 {
		t.Fatal("did not advance")
	}
	if _, g, _, _ := m.source.At(0, 0).RGBA(); g != 65535 {
		t.Fatal("wrong frame displayed")
	}
	stepAnimation(t, m)
	if !a.finished || m.scheduleAnimation() != nil {
		t.Fatal("finite loop did not stop")
	}
	if !strings.Contains(m.detail(80, 20), "finished") || !strings.Contains(m.hints(), "Space pause/play") {
		t.Fatal("playback state or controls missing")
	}
	cmd := m.toggleAnimation()
	m.animationFrame(cmd().(animationFrame))
	if a.finished || a.player.Frame() != 1 {
		t.Fatal("Space did not restart finished GIF")
	}
	// Pause even a frame already being composed; its completion cannot advance.
	pending := m.animationTick(animationTick{a, a.epoch})
	m.toggleAnimation()
	m.animationFrame(pending().(animationFrame))
	if a.player.Frame() != 1 {
		t.Fatal("late composition changed paused frame")
	}
}

func TestGIFHiddenReloadAndReplacementStopOldWork(t *testing.T) {
	for _, action := range []string{"help", "browser", "menu", "reload", "file", "quit", "close"} {
		t.Run(action, func(t *testing.T) {
			m := animated(t, "glyph", 0)
			a := m.animation
			timer := m.scheduleAnimation()
			pending := m.animationTick(animationTick{a, a.epoch})
			switch action {
			case "help":
				m.Update(press("?"))
			case "browser":
				m.screen = screenBrowser
				m.scheduleAnimation()
			case "menu":
				m.openMenu(0)
			case "reload":
				m.load(true)
			case "file":
				m.switchFile(1)
			case "quit":
				m.quit()
			case "close":
				m.Close()
			}
			if timer() != nil {
				t.Fatal("long timer survived leaving the document")
			}
			before := m.source
			m.animationFrame(pending().(animationFrame))
			if m.source != before {
				t.Fatal("late frame replaced hidden/new document")
			}
			if action == "help" || action == "browser" {
				m.help = false
				m.screen = screenDocument
				if m.scheduleAnimation() == nil {
					t.Fatal("visible GIF did not resume")
				}
			} else if a.player != nil {
				t.Fatal("abandoned timer retained decoded frames")
			}
		})
	}
}

func TestGIFExportCapturesDisplayedFrame(t *testing.T) {
	m := animated(t, "glyph", 0)
	stepAnimation(t, m)
	var out saved
	m.opts.Save = out.save(t)
	cmd := m.export()
	stepAnimation(t, m) // The export command has not run yet; playback continues.
	m.Update(cmd())
	if len(out.images) != 1 {
		t.Fatal("no exported frame")
	}
	if r, g, _, _ := out.images[0].At(0, 0).RGBA(); r != 0 || g != 65535 {
		t.Fatal("export did not snapshot the displayed green frame")
	}
	if r, _, _, _ := m.source.At(0, 0).RGBA(); r != 65535 {
		t.Fatal("export disturbed playback")
	}
	if m.exportRequest().Animate {
		t.Fatal("headless export enabled animation")
	}
}

func TestGIFKittyBackpressureResizeAndCleanup(t *testing.T) {
	m := animated(t, "kitty", 0)
	a := m.animation
	first := m.picID
	draw := stepAnimation(t, m)
	if !a.awaiting || m.scheduleAnimation() != nil || m.picID == first {
		t.Fatal("Kitty failed to wait for its fresh frame")
	}
	a.paused = true
	pump(m, draw, 0)
	if a.awaiting {
		t.Fatal("transmission did not acknowledge presentation")
	}
	_, resize := m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	pump(m, resize, 0)
	if m.pic.Mode() != picture.PictureKitty {
		t.Fatal("resize lost renderer")
	}
	a.paused = false
	if m.scheduleAnimation() == nil {
		t.Fatal("rendered frame did not start its delay")
	}
	// Accept a frame, then leave before its raw transmission executes.
	a.cancelPending()
	render := m.pic.SetImage(m.source)
	frame, ok := render().(picture.KittyFrameMsg)
	if !ok {
		t.Fatal("missing Kitty frame")
	}
	accepted := m.animationTransmission(frame, m.pic.Update(frame))
	flight := m.animationPlacement
	id := m.picID
	cleanup := m.clearGraphics()
	if flight.active.Load() {
		t.Fatal("old placement still active")
	}
	pump(m, cleanup, 0)
	// Extract the sequence's final command to verify late cleanup is ID-specific.
	// The generic runner handles all nested picture transmission commands too.
	var raw string
	var run func(tea.Cmd)
	run = func(cmd tea.Cmd) {
		if cmd == nil {
			return
		}
		msg := cmd()
		if cmds, ok := msg.(tea.BatchMsg); ok {
			for _, c := range cmds {
				run(c)
			}
			return
		}
		v := reflect.ValueOf(msg)
		if v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeOf(tea.Cmd(nil)) {
			for i := 0; i < v.Len(); i++ {
				run(v.Index(i).Interface().(tea.Cmd))
			}
			return
		}
		if s, ok := msg.(tea.RawMsg); ok {
			raw += fmt.Sprint(s.Msg)
		}
	}
	run(accepted)
	if !strings.HasSuffix(raw, fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", id)) {
		t.Fatalf("late frame was not deleted: %q", raw)
	}
}

func TestGIFImageIDsUseReservedSlots(t *testing.T) {
	m := animated(t, "kitty", 0)
	a := m.animation
	seen := map[int]bool{}
	for range 2100 {
		id := a.nextImageID()
		if id <= 0 || id >= 1<<24 || seen[id] {
			t.Fatal("invalid or repeated animation ID")
		}
		seen[id] = true
		if len(seen) == 500 {
			seen[nextKittyID()] = true
		} // Another component allocates a block.
	}
}

func TestGIFLongTimerCancellationIsPrompt(t *testing.T) {
	m := animated(t, "glyph", 0)
	timer := m.scheduleAnimation()
	done := make(chan tea.Msg, 1)
	go func() { done <- timer() }()
	m.stopAnimation()
	select {
	case msg := <-done:
		if msg != nil {
			t.Fatal("cancel emitted a tick")
		}
	case <-time.After(time.Second):
		t.Fatal("long frame delay blocked cleanup")
	}
}

// Exercise scheduling through Bubble Tea itself, including nested transmission
// sequences and the presentation acknowledgement, instead of injecting ticks.
type animationObserver struct {
	*Model
	reached   bool
	flickered bool
}

func (m *animationObserver) Init() tea.Cmd {
	return tea.Batch(m.Model.Init(), func() tea.Msg { return tea.WindowSizeMsg{Width: 30, Height: 12} })
}
func (m *animationObserver) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.Model.Update(msg)
	if m.animation != nil && m.pic.Mode() == picture.PictureKitty && glyphs(m.pictureView()) {
		m.flickered = true
	}
	if m.animation != nil && m.animation.player.Frame() >= 3 {
		m.reached = true
		return m, m.quit()
	}
	return m, cmd
}
func TestGIFActualEventLoop(t *testing.T) {
	for _, render := range []string{"glyph", "kitty"} {
		t.Run(render, func(t *testing.T) {
			t.Cleanup(func() { picture.ForceKittyCapability(picture.KittyCapabilityUnknown) })
			model := &animationObserver{Model: New(Options{Files: []string{"motion.gif"}, FilesFS: examples.Files, Render: render, Page: 1})}
			defer model.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err := tea.NewProgram(model, tea.WithContext(ctx), tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler()).Run()
			if err != nil || !model.reached || model.flickered {
				t.Fatalf("%s playback stalled: %v, note %s", render, err, model.note+fmt.Sprintf(" flickered=%v", model.flickered))
			}
		})
	}
}

func TestGIFKittyKeepsFrontUntilReplacementIsPresented(t *testing.T) {
	m := animated(t, "kitty", 0)
	a, front := m.animation, m.animationFront
	if front == nil || glyphs(m.pictureView()) {
		t.Fatal("initial Kitty frame was not presented")
	}
	draw := stepAnimation(t, m)
	a.paused = true // Run rendering commands without scheduling another frame.
	if m.pictureView() != front.view || !front.flight.active.Load() {
		t.Fatal("discarded visible frame while its replacement was encoding")
	}
	// An additional zoom during encoding must replace only the unfinished back
	// picture, keeping the same front image and rejecting the obsolete result.
	obsolete := m.animationPlacement
	m.zoom = 1
	zoom := m.refreshImage()
	if obsolete.active.Load() || m.animationFront != front || m.pictureView() != front.view {
		t.Fatal("zoom discarded the front or retained the obsolete back")
	}
	pump(m, draw, 0)
	if m.animationFront != front || m.pictureView() != front.view {
		t.Fatal("obsolete frame was displayed")
	}
	pump(m, zoom, 0)
	if m.animationFront == front || m.animationFront.id != m.picID || glyphs(m.pictureView()) || front.flight.active.Load() {
		t.Fatal("replacement failed to swap Kitty grids and retire the old image")
	}
}

func TestGIFPendingKittyFrameCleanupAndKeepScreen(t *testing.T) {
	for _, action := range []string{"quit", "keep", "reload", "menu", "glyph"} {
		t.Run(action, func(t *testing.T) {
			m := animated(t, "kitty", 0)
			front := m.animationFront
			draw := stepAnimation(t, m)
			pending := m.animationPlacement
			var cleanup tea.Cmd
			switch action {
			case "quit":
				cleanup = m.quit()
			case "keep":
				m.opts.KeepScreen = true
				cleanup = m.quit()
			case "reload":
				m.load(true)
				result := m.loader.Load(m.request(true))
				cleanup = m.loaded(result)
				m.animation.paused = true
			case "menu":
				cleanup = m.openMenu(0)
			case "glyph":
				_, cleanup = m.Update(press("g"))
			}
			if action == "keep" {
				if m.picID != front.id || m.pictureView() != front.view || m.source != front.source || !front.flight.active.Load() {
					t.Fatal("keep-screen did not preserve the visible frame")
				}
			} else if front.flight.active.Load() {
				t.Fatal("abandoned front image was not retired")
			}
			if pending.active.Load() {
				t.Fatal("unfinished image was not retired")
			}
			pump(m, cleanup, 0)
			pump(m, draw, 0) // Late render cannot restore a discarded image or old front.
			if action == "glyph" {
				if m.animationFront != nil || !glyphs(m.pictureView()) {
					t.Fatal("explicit glyph mode did not take effect")
				}
				_, cmd := m.Update(press("g"))
				m.animation.paused = true
				if glyphs(m.pictureView()) {
					t.Fatal("switch to Kitty exposed a transitional glyph frame")
				}
				pump(m, cmd, 0)
				if m.animationFront == nil || glyphs(m.pictureView()) {
					t.Fatal("switch back to Kitty did not present an image")
				}
			}
		})
	}
}

func TestGIFKittyExportUsesFrontWhileBackIsEncoding(t *testing.T) {
	m := animated(t, "kitty", 0)
	front := m.animationFront
	stepAnimation(t, m) // Green is composed but has not replaced the red front.
	var out saved
	m.opts.Save = out.save(t)
	result := m.export()()
	if len(out.images) != 1 {
		t.Fatal("no export")
	}
	if r, g, _, _ := out.images[0].At(0, 0).RGBA(); r != 65535 || g != 0 {
		t.Fatal("export captured an undisplayed frame")
	}
	m.Update(result)
	if m.animationFront != front {
		t.Fatal("export disturbed presentation")
	}
}

func assertAnimationFits(t *testing.T, m *Model) {
	t.Helper()
	view := m.View().Content
	if lines := strings.Count(view, "\n") + 1; lines > m.height {
		t.Fatalf("view has %d rows in a %d-row window", lines, m.height)
	}
	for _, line := range strings.Split(view, "\n") {
		if width := ansi.StringWidth(line); width > m.width {
			t.Fatalf("view has %d columns in a %d-column window", width, m.width)
		}
	}
	if glyphs(m.pictureView()) {
		t.Fatal("resize exposed transitional glyphs")
	}
}

func TestGIFKittyResizeBurstRejectsOldPresentations(t *testing.T) {
	m := animated(t, "kitty", 0)
	a, front := m.animation, m.animationFront
	// A transmission accepted before resizing can complete after a new layout.
	frame := m.pic.SetImage(m.source)().(picture.KittyFrameMsg)
	accepted := m.animationTransmission(frame, m.pic.Update(frame))
	a.paused = true
	var renders []tea.Cmd
	var flights []*animationPlacement
	for _, size := range []tea.WindowSizeMsg{
		{Width: 25, Height: 8}, {Width: 8, Height: 4},
		{Width: 1, Height: 1}, {Width: 60, Height: 18},
	} {
		oldID := m.picID
		_, cmd := m.Update(size)
		if m.picID == oldID || !a.awaiting || m.animationFront != front {
			t.Fatal("resize did not retain the front and prepare a new placement")
		}
		assertAnimationFits(t, m)
		renders = append(renders, cmd)
		flights = append(flights, m.animationPlacement)
		m.Update(size) // Repeated layout must not invalidate work already in flight.
		if m.animationPlacement != flights[len(flights)-1] {
			t.Fatal("unchanged dimensions restarted rendering")
		}
	}
	pump(m, accepted, 0)
	if m.animationFront != front || !a.awaiting {
		t.Fatal("old presentation acknowledged the new geometry")
	}
	// Newest completes first; older encodes then arrive out of order.
	pump(m, renders[len(renders)-1], 0)
	current := m.animationFront
	for i := len(renders) - 2; i >= 0; i-- {
		pump(m, renders[i], 0)
		if flights[i].active.Load() || m.animationFront != current {
			t.Fatal("obsolete resize survived or replaced the latest frame")
		}
	}
	if current == front || front.flight.active.Load() || a.awaiting || !a.paused {
		t.Fatal("resize failed to settle, clean up, or preserve pause")
	}
	assertAnimationFits(t, m)
	a.paused = false
	if m.scheduleAnimation() == nil {
		t.Fatal("playback did not resume after resizing")
	}
}

func TestGIFKittyFontResizeCancelsPendingComposition(t *testing.T) {
	m := animated(t, "kitty", 0)
	a, front := m.animation, m.animationFront
	tick := animationTick{a, a.epoch}
	composed := m.animationTick(tick)()
	_, resize := m.Update(uv.CellSizeEvent{Width: 11, Height: 23})
	if m.picID == front.id || a.epoch == tick.epoch || !a.awaiting {
		t.Fatal("font resize reused the old placement or composition")
	}
	_, cmd := m.Update(composed)
	if cmd != nil || a.player.Frame() != 1 {
		t.Fatal("old composition advanced playback during resize")
	}
	a.paused = true
	pump(m, resize, 0)
	if w, h := m.pic.CellPixelSize(); w != 11 || h != 23 {
		t.Fatalf("lost font geometry: %dx%d", w, h)
	}
	if m.animationFront == front || a.awaiting {
		t.Fatal("font resize failed to present")
	}
	assertAnimationFits(t, m)
}

func TestGIFSpeedKeysAndDelays(t *testing.T) {
	for _, render := range []string{"glyph", "kitty"} {
		t.Run(render, func(t *testing.T) {
			m := animated(t, render, 0)
			a := m.animation
			a.paused = true
			if a.rate() != 1 || a.delay() != 655350*time.Millisecond {
				t.Fatal("new GIF does not start at its original speed")
			}
			for _, tt := range []struct {
				key  string
				rate float64
			}{
				{"<", .5}, {"<", .25}, {"<", .25},
				{">", .5}, {">", 1}, {">", 2}, {">", 4}, {">", 4},
				{"backspace", 1},
			} {
				before, id := m.source, m.picID
				m.Update(press(tt.key))
				if a.rate() != tt.rate || !a.paused || a.pending || m.source != before || m.picID != id {
					t.Fatalf("%s: rate %g, paused %v, pending %v", tt.key, a.rate(), a.paused, a.pending)
				}
				want := time.Duration(float64(655350*time.Millisecond) / tt.rate)
				if a.delay() != want || !strings.Contains(m.detail(120, 40), fmt.Sprintf("%g× speed", tt.rate)) {
					t.Fatalf("wrong delay or speed display at %g×", tt.rate)
				}
			}
			// A 20 ms encoded delay can run at 5 ms; it is not renormalized to
			// 100 ms after scaling. Zoom and layout do not change playback speed.
			a.player, _ = a.player.Next()
			m.Update(press(">"))
			m.Update(press(">"))
			if a.delay() != 5*time.Millisecond {
				t.Fatalf("fast short frame: %v", a.delay())
			}
			_, cmd := m.Update(press("+"))
			pump(m, cmd, 0)
			_, cmd = m.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
			pump(m, cmd, 0)
			if a.rate() != 4 || !a.paused || m.zoom != 1 {
				t.Fatal("zoom/resize changed playback settings")
			}
			// Reload and file navigation create a new playback session at 1×.
			m.load(true)
			cmd = m.loaded(m.loader.Load(m.request(true)))
			m.animation.paused = true
			pump(m, cmd, 0)
			if m.animation.rate() != 1 {
				t.Fatal("new playback inherited speed from the previous session")
			}
		})
	}
}

func TestGIFSpeedChangeCancelsTimerAndStaleComposition(t *testing.T) {
	m := animated(t, "glyph", 0)
	a := m.animation
	timer := m.scheduleAnimation()
	tick := animationTick{a, a.epoch}
	composed := m.animationTick(tick)()
	_, replacement := m.Update(press(">"))
	if timer() != nil || a.epoch == tick.epoch || !a.pending || replacement == nil {
		t.Fatal("speed change did not replace the current delay")
	}
	m.Update(composed)
	if a.player.Frame() != 1 || a.rate() != 2 {
		t.Fatal("obsolete composition advanced at the previous speed")
	}
	m.Update(press("space"))
	pump(m, replacement, 0) // The replaced long timer must now be cancelled too.
	if !a.paused || a.pending {
		t.Fatal("pause failed after speed change")
	}
}

func TestGIFSpeedPreservesKittyBackpressureAndFinitePlayback(t *testing.T) {
	m := animated(t, "kitty", -1)
	a := m.animation
	draw := stepAnimation(t, m)
	m.Update(press(">"))
	if !a.awaiting || a.pending || m.scheduleAnimation() != nil {
		t.Fatal("speed change bypassed Kitty backpressure")
	}
	a.paused = true
	pump(m, draw, 0)
	a.paused = false
	stepAnimation(t, m)
	if !a.finished {
		t.Fatal("finite playback did not finish")
	}
	m.Update(press("<"))
	if !a.finished || a.pending || a.rate() != 1 {
		t.Fatal("speed change restarted finished playback")
	}
	m.Update(press("<"))
	restart := m.toggleAnimation()
	draw = m.animationFrame(restart().(animationFrame))
	a.paused = true
	pump(m, draw, 0)
	if a.rate() != .5 || a.finished || a.player.Frame() != 1 {
		t.Fatal("restart lost the selected speed")
	}
}
