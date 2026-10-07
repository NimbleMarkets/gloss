package app

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/charmbracelet/x/ansi"
)

const (
	maxSearchRunes = 256
	maxSearchHits  = 1000
	maxSearchBytes = 16 << 20
	searchTimeout  = 30 * time.Second
)

type searchPattern = regexp.Regexp

// A match is one displayed line, visible-column cell, or PDF page. Every
// occurrence on the line/cell is highlighted; PDF results carry an excerpt.
type textMatch struct {
	row, col, page int
	excerpt        string
}

type searchWorker struct{ sync.Mutex }

type textSearch struct {
	path, query      string
	draft            []rune
	cursor           int
	editing, running bool
	pattern          *searchPattern
	matches          []textMatch
	at               int
	note, unit       string
	revision         uint64
	cancel           context.CancelFunc
	markdown         *markdownView
	layout           uint64
	sheet            *sheetView
	columns          []int
}

type searchResult struct {
	owner    *textSearch
	revision uint64
	matches  []textMatch
	note     string
}

func (m *Model) searchable() bool {
	return !m.isPreview && !m.loading && m.err == nil && len(m.opts.Files) > 0 &&
		(m.markdown != nil || m.sheet != nil || m.kind == "pdf")
}

func (m *Model) editSearch() {
	if m.search == nil {
		m.search = &textSearch{path: m.opts.Files[m.index], at: -1}
	}
	s := m.search
	s.editing, s.draft = true, []rune(s.query)
	s.cursor = len(s.draft)
}

func (m *Model) clearSearch() {
	if m.search != nil && m.search.cancel != nil {
		m.search.cancel()
	}
	if m.markdown != nil {
		m.markdown.search = nil
	}
	if m.sheet != nil {
		m.sheet.search = nil
	}
	m.search = nil
}

func (s *textSearch) insert(text string) {
	var clean []rune
	for _, r := range text {
		if len(s.draft)+len(clean) >= maxSearchRunes {
			break
		}
		if unicode.IsSpace(r) {
			r = ' '
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			continue
		}
		clean = append(clean, r)
	}
	s.draft = slices.Insert(s.draft, s.cursor, clean...)
	s.cursor += len(clean)
}

func (m *Model) searchKey(msg tea.KeyPressMsg) tea.Cmd {
	s := m.search
	switch msg.String() {
	case "ctrl+c":
		return m.quit()
	case "esc":
		s.editing = false
		if s.query == "" {
			m.clearSearch()
		}
	case "enter":
		s.editing, s.query = false, string(s.draft)
		if s.query == "" {
			m.clearSearch()
			return nil
		}
		s.pattern = regexp.MustCompile("(?i)" + regexp.QuoteMeta(s.query))
		return m.startSearch()
	case "left":
		s.cursor = max(0, s.cursor-1)
	case "right":
		s.cursor = min(len(s.draft), s.cursor+1)
	case "home", "ctrl+a":
		s.cursor = 0
	case "end", "ctrl+e":
		s.cursor = len(s.draft)
	case "backspace":
		if s.cursor > 0 {
			s.draft = slices.Delete(s.draft, s.cursor-1, s.cursor)
			s.cursor--
		}
	case "delete":
		if s.cursor < len(s.draft) {
			s.draft = slices.Delete(s.draft, s.cursor, s.cursor+1)
		}
	case "ctrl+u":
		s.draft, s.cursor = nil, 0
	case "ctrl+w":
		end := s.cursor
		for s.cursor > 0 && unicode.IsSpace(s.draft[s.cursor-1]) {
			s.cursor--
		}
		for s.cursor > 0 && !unicode.IsSpace(s.draft[s.cursor-1]) {
			s.cursor--
		}
		s.draft = slices.Delete(s.draft, s.cursor, end)
	default:
		s.insert(msg.Text)
	}
	return nil
}

// Reindex after layout/source or visible-column changes. Workers only read
// immutable snapshots, and both ownership and revision guard their replies.
func (m *Model) syncSearch() tea.Cmd {
	s := m.search
	if s == nil {
		return nil
	}
	if m.screen != screenDocument || len(m.opts.Files) == 0 || m.opts.Files[m.index] != s.path || m.quitting {
		m.clearSearch()
		return nil
	}
	if m.loading {
		return nil
	}
	if !m.searchable() {
		m.clearSearch()
		return nil
	}
	if s.pattern == nil {
		return nil
	}
	if m.markdown != s.markdown || m.sheet != s.sheet ||
		(m.markdown != nil && m.markdown.layoutVersion != s.layout) ||
		(m.sheet != nil && !slices.Equal(m.sheet.cols, s.columns)) {
		return m.startSearch()
	}
	return nil
}

func (m *Model) startSearch() tea.Cmd {
	s := m.search
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	s.cancel = cancel
	s.revision++
	s.running, s.matches, s.at, s.note = true, nil, -1, ""
	s.markdown, s.sheet = m.markdown, m.sheet
	var lines []markdownLine
	var sheet *document.Sheet
	var columns []int
	if m.markdown != nil {
		s.layout = m.markdown.layoutVersion
		m.markdown.search = s.pattern
		lines, s.unit = m.markdown.lines, "lines"
	}
	if m.sheet != nil {
		m.sheet.search = s.pattern
		sheet = m.sheet.sheet
		columns = slices.Clone(m.sheet.cols)
		s.columns, s.unit = columns, "cells"
	}
	if m.kind == "pdf" {
		s.unit = "pages"
	}
	request, files, pages := m.request(false), m.opts.FilesFS, m.pages
	request.TextOnly, request.Animate = true, false
	pattern, revision := s.pattern, s.revision
	if m.searchWorker == nil {
		m.searchWorker = &searchWorker{}
	}
	worker := m.searchWorker
	return func() tea.Msg {
		defer cancel()
		// Serial across query replacements, including canceled PDF extraction.
		worker.Lock()
		defer worker.Unlock()
		out := searchResult{owner: s, revision: revision}
		used := 0
		stopped := func() bool {
			if ctx.Err() != nil {
				out.note = "search stopped (30s limit)"
				return true
			}
			if used >= maxSearchBytes || len(out.matches) >= maxSearchHits {
				out.note = "partial results: search limit reached"
				return true
			}
			return false
		}
		add := func(text string, hit textMatch) {
			truncated := false
			if len(text) > maxSearchBytes-used {
				truncated = true
				text = text[:maxSearchBytes-used]
				for !utf8.ValidString(text) {
					text = text[:len(text)-1]
				}
			}
			used += len(text)
			if loc := pattern.FindStringIndex(text); loc != nil {
				hit.excerpt = searchExcerpt(text, loc[0], loc[1])
				out.matches = append(out.matches, hit)
			}
			if truncated {
				used = maxSearchBytes
				out.note = "partial results: search limit reached"
			}
		}
		switch {
		case lines != nil:
			for row, line := range lines {
				if stopped() {
					break
				}
				if line.image < 0 {
					add(ansi.Strip(line.text), textMatch{row: row})
				}
			}
		case sheet != nil:
		rows:
			for row, cells := range sheet.Rows {
				for _, col := range columns {
					if stopped() {
						break rows
					}
					if col < len(cells) {
						add(safe(cells[col]), textMatch{row: row, col: col})
					}
				}
			}
		default:
			loader := &document.Loader{Files: files}
			defer loader.Close()
			skipped := 0
			for page := 1; page <= pages; page++ {
				if stopped() {
					break
				}
				request.Page = page
				result := loader.Load(request)
				if ctx.Err() != nil {
					stopped()
					break
				}
				if result.Err != nil {
					if !errors.Is(result.Err, document.ErrNoTextLayer) {
						out.note = fmt.Sprintf("partial results: page %d: %s", page, safe(result.Err.Error()))
						break
					}
					skipped++
					continue
				}
				add(result.Text, textMatch{page: page})
			}
			if skipped > 0 {
				out.note += fmt.Sprintf(" · %d pages without text skipped", skipped)
			}
		}
		return out
	}
}

func (m *Model) searched(result searchResult) tea.Cmd {
	s := m.search
	if s == nil || s != result.owner || s.revision != result.revision {
		return nil
	}
	s.matches, s.note, s.running = result.matches, result.note, false
	// Start at the current reading position, wrapping if it has no later hit.
	for i, hit := range s.matches {
		if (m.markdown != nil && hit.row >= m.markdown.offset) ||
			(m.sheet != nil && (hit.row > m.sheet.at[0] || hit.row == m.sheet.at[0] && hit.col >= m.sheet.column())) ||
			(m.kind == "pdf" && hit.page >= m.page) {
			s.at = i - 1
			break
		}
	}
	return m.nextMatch(1)
}

func (m *Model) nextMatch(delta int) tea.Cmd {
	s := m.search
	if len(s.matches) == 0 {
		return nil
	}
	s.at = (s.at + delta + len(s.matches)) % len(s.matches)
	hit := s.matches[s.at]
	switch {
	case m.markdown != nil:
		m.markdown.offset = hit.row
		m.markdown.scroll(0)
	case m.sheet != nil:
		m.sheet.at = [2]int{hit.row, m.sheet.place(hit.col)}
		m.sheet.settle(m.width, m.bodyHeight()-1)
	case hit.page > 0:
		if hit.page != m.page {
			return m.movePage(hit.page)
		}
	}
	return nil
}

func (s *textSearch) status(width int) string {
	if s.editing {
		left := string(s.draft[:s.cursor])
		input := left + "\x1b[7m▏\x1b[27m" + string(s.draft[s.cursor:])
		start := max(0, ansi.StringWidth(left)-max(1, width-3))
		return ansi.Truncate("/"+ansi.Cut(input, start, start+max(1, width-1))+"  Enter search · Esc cancel", width, "")
	}
	prefix := "/" + ansi.Truncate(s.query, max(4, min(20, width/4)), "…") + " · "
	if s.running {
		return prefix + "searching… · Esc cancel"
	}
	count := "no matches"
	if len(s.matches) > 0 {
		count = fmt.Sprintf("%d/%d %s", s.at+1, len(s.matches), s.unit)
	}
	text := prefix + count
	if s.note != "" {
		text += " · " + s.note
	}
	text += " · n/N · Esc clear"
	if s.at >= 0 && s.at < len(s.matches) {
		text += " · " + s.matches[s.at].excerpt
	}
	return text
}

func searchExcerpt(text string, start, end int) string {
	// Slice before converting to runes: a PDF page can contain 16 MiB of text.
	lo, hi := max(0, start-80), min(len(text), end+240)
	for lo < start && !utf8.RuneStart(text[lo]) {
		lo++
	}
	for hi < len(text) && !utf8.RuneStart(text[hi]) {
		hi--
	}
	left, right := []rune(text[lo:start]), []rune(text[end:hi])
	prefix, suffix := "", ""
	if lo > 0 {
		prefix = "…"
	}
	if hi < len(text) {
		suffix = "…"
	}
	if len(left) > 20 {
		left, prefix = left[len(left)-20:], "…"
	}
	if len(right) > 60 {
		right, suffix = right[:60], "…"
	}
	return prefix + strings.Join(strings.Fields(safe(string(left)+text[start:end]+string(right))), " ") + suffix
}

// Walk graphemes and escapes separately so Unicode widths and OSC 8 links
// survive. Restore the original SGR state after each highlighted span, and
// reapply the highlight after syntax-color resets within a match.
func highlightSearch(text string, pattern *searchPattern) string {
	if pattern == nil {
		return text
	}
	hits := pattern.FindAllStringIndex(ansi.Strip(text), -1)
	if len(hits) == 0 {
		return text
	}
	const mark = "\x1b[30;103m"
	var out, style strings.Builder
	state, pos, at := byte(0), 0, 0
	marked := false
	for len(text) > 0 {
		seq, width, n, next := ansi.DecodeSequence(text, state, nil)
		state, text = next, text[n:]
		if width == 0 && strings.HasPrefix(seq, "\x1b") {
			out.WriteString(seq)
			if strings.HasPrefix(seq, "\x1b[") && strings.HasSuffix(seq, "m") {
				if seq == "\x1b[0m" || seq == "\x1b[m" {
					style.Reset()
				}
				style.WriteString(seq)
				if marked {
					out.WriteString(mark)
				}
			}
			continue
		}
		for at < len(hits) && hits[at][1] <= pos {
			at++
		}
		active := at < len(hits) && hits[at][0] < pos+len(seq)
		if active && !marked {
			out.WriteString(mark)
		}
		if !active && marked {
			out.WriteString("\x1b[0m")
			out.WriteString(style.String())
		}
		marked = active
		out.WriteString(seq)
		pos += len(seq)
	}
	if marked {
		out.WriteString("\x1b[0m")
		out.WriteString(style.String())
	}
	return out.String()
}
