package app

import (
	"context"
	"image"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/NimbleMarkets/ntcharts/v2/picture"
	"github.com/charmbracelet/x/ansi"
)

type animation struct {
	cancelTimer                         context.CancelFunc
	player                              *document.GIFPlayer
	epoch                               uint64
	paused, finished, pending, awaiting bool
	id, remaining                       int
	speed                               int // Powers of two: -2..2, default 0 = 1×.
}

func (a *animation) rate() float64 { return float64(int(1)<<uint(a.speed+2)) / 4 }

func (a *animation) delay() time.Duration {
	// GIFPlayer has already normalized tiny/zero delays. Even at 4× the
	// shortest delay is 5 ms, and the largest at 0.25× fits time.Duration.
	return time.Duration(float64(a.player.Delay()) / a.rate())
}

func (a *animation) setSpeed(speed int) {
	speed = max(-2, min(2, speed))
	if speed == a.speed {
		return
	}
	a.cancelPending()
	a.speed = speed
	// The Update defer schedules the current frame's new delay. Paused,
	// finished, hidden, and still-transmitting animations remain stopped.
}

type animationTick struct {
	owner *animation
	epoch uint64
}
type animationFrame struct {
	animationTick
	player *document.GIFPlayer
	more   bool
}
type animationPresented struct {
	owner *animation
	id    int
}

// A frame already accepted by picture may still be transmitting when the
// user leaves. Retire it again after that transmission, just as QR does.
type animationPlacement struct {
	active  atomic.Bool
	cleanup tea.Cmd
}

// Keep the last transmitted picture while the next picture encodes. Each
// frame still gets a fresh ID for ghostty-web; its predecessor is deleted only
// after the replacement's placeholder grid can be displayed.
type animationPicture struct {
	pic    picture.Model
	id     int
	view   string
	source image.Image
	flight *animationPlacement
}

func retireAnimationPlacement(flight *animationPlacement, cleanup tea.Cmd) {
	if flight != nil && flight.active.Load() {
		flight.cleanup = cleanup
		flight.active.Store(false)
	}
}

func (m *Model) retireAnimationPicture(cleanup tea.Cmd) {
	if m.animationPlacement != nil {
		retireAnimationPlacement(m.animationPlacement, cleanup)
		m.animationPlacement = nil
	}
}

func (m *Model) clearAnimationFront() tea.Cmd {
	front := m.animationFront
	if front == nil {
		return nil
	}
	m.animationFront = nil
	cleanup := front.pic.SetImage(nil)
	retireAnimationPlacement(front.flight, cleanup)
	if m.animationPlacement == front.flight {
		m.animationPlacement = nil
	}
	return cleanup
}

func (m *Model) presentAnimationPicture() tea.Cmd {
	var cleanup tea.Cmd
	if m.animationFront != nil && m.animationFront.id != m.picID {
		cleanup = m.clearAnimationFront()
	}
	m.animationFront = &animationPicture{pic: m.pic, id: m.picID, view: m.pic.View().Content, source: m.source, flight: m.animationPlacement}
	m.animation.awaiting = false
	return cleanup
}

func (m *Model) pictureView() string {
	if m.animation != nil && m.pic.Mode() == picture.PictureKitty {
		if m.animationFront != nil {
			// A retained grid belongs to its original placement geometry. Clip
			// its placeholders while a resize encodes; wrapping them would move
			// image rows and push the status bar out of the new viewport.
			lines := strings.Split(m.animationFront.view, "\n")
			lines = lines[:min(len(lines), m.bodyHeight())]
			for i := range lines {
				lines[i] = ansi.Truncate(lines[i], max(1, m.width), "")
			}
			return strings.Join(lines, "\n")
		}
		return ansi.Truncate("Preparing animation…", max(1, m.width), "")
	}
	return m.pic.View().Content
}

// Resize uses a fresh picture just like a new frame. Reusing the front's ID
// would replace its terminal placement before the new grid is ready. It would
// also let an acknowledgement from the previous geometry present a stale grid.
func (m *Model) resizeAnimation() tea.Cmd {
	m.animation.cancelPending()
	return m.refreshImage()
}

// Keep-screen quit leaves the frame actually on screen, discarding an
// unfinished replacement. Ownership of that visible image passes to the TTY.
func (m *Model) keepAnimationPicture() tea.Cmd {
	front := m.animationFront
	if front == nil || m.pic.Mode() != picture.PictureKitty {
		return m.clearAnimationFront()
	}
	var cleanup tea.Cmd
	if front.id != m.picID {
		cleanup = m.pic.SetImage(nil)
		m.retireAnimationPicture(cleanup)
	}
	m.pic, m.picID = front.pic, front.id
	m.source = front.source
	m.animationFront, m.animationPlacement = nil, nil
	return cleanup
}

func (m *Model) stopAnimation() {
	if a := m.animation; a != nil {
		a.cancelPending()
		a.player = nil // A late timer must not retain decoded frames.
	}
	m.animation = nil
}

func (a *animation) cancelPending() {
	if a.cancelTimer != nil {
		a.cancelTimer()
		a.cancelTimer = nil
	}
	a.epoch++
	a.pending = false
}

func (m *Model) animationVisible() bool {
	return m.animation != nil && !m.isPreview && !m.loading && !m.suspended && !m.quitting && !m.help && m.screen == screenDocument
}

// One timer or compositor command at a time. Kitty starts the next delay only
// after the preceding picture has been transmitted, preventing render backlog.
// Hidden documents cancel their timers without retaining decoded frames.
func (m *Model) scheduleAnimation() tea.Cmd {
	a := m.animation
	if !m.animationVisible() {
		if a != nil && a.pending {
			a.cancelPending()
		}
		return nil
	}
	if a.paused || a.finished || a.pending {
		return nil
	}
	if a.awaiting && m.pic.Mode() == picture.PictureKitty {
		return nil
	}
	a.pending = true
	tick := animationTick{a, a.epoch}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancelTimer = cancel
	delay := a.delay()
	return func() tea.Msg {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			return tick
		}
	}
}

func (m *Model) animationTick(t animationTick) tea.Cmd {
	a := m.animation
	if a == nil || a != t.owner || a.epoch != t.epoch {
		return nil
	}
	if !m.animationVisible() || a.paused || a.finished || (a.awaiting && m.pic.Mode() == picture.PictureKitty) {
		a.pending = false
		return nil
	}
	player := a.player // Immutable; composition stays off the event loop.
	return func() tea.Msg { next, more := player.Next(); return animationFrame{t, next, more} }
}

func (m *Model) animationFrame(f animationFrame) tea.Cmd {
	a := m.animation
	if a == nil || a != f.owner || a.epoch != f.epoch {
		return nil
	}
	a.pending = false
	if !m.animationVisible() || a.paused {
		return nil
	}
	if !f.more {
		a.finished = true
		return nil
	}
	a.player = f.player
	m.source = f.player.Image()
	return m.refreshImage()
}

func (m *Model) toggleAnimation() tea.Cmd {
	a := m.animation
	a.cancelPending()
	if a.finished {
		a.finished, a.paused = false, false
		// Restart composition is also a command, so large canvases don't block keys.
		a.pending = true
		tick, player := animationTick{a, a.epoch}, a.player
		return func() tea.Msg { return animationFrame{tick, player.Restart(), true} }
	}
	a.paused = !a.paused
	return nil
}

// Each host allocation reserves 1,000 IDs. Consume those reserved slots for
// animation instead of burning a whole block for every frame. Other pictures,
// previews, charts, and QR components continue to use the shared allocator.
func (a *animation) nextImageID() int {
	if a.remaining == 0 {
		a.id, a.remaining = nextKittyID(), 1000
	}
	id := a.id
	a.id++
	a.remaining--
	return id
}

func (m *Model) animationTransmission(msg tea.Msg, cmd tea.Cmd) tea.Cmd {
	if frame, ok := msg.(picture.KittyFrameMsg); !ok || frame.ID != m.picID || cmd == nil || m.animation == nil {
		return cmd
	}
	if m.animationPlacement == nil {
		m.animationPlacement = &animationPlacement{}
		m.animationPlacement.active.Store(true)
	}
	a, id, flight := m.animation, m.picID, m.animationPlacement
	return tea.Sequence(cmd, func() tea.Msg {
		if flight != nil && !flight.active.Load() && flight.cleanup != nil {
			return flight.cleanup()
		}
		return animationPresented{a, id}
	})
}
