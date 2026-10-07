package app

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/charmbracelet/x/ansi"
)

func searchDocument(t *testing.T, text string) *Model {
	t.Helper()
	m := New(Options{Files: []string{"text.md"}, Render: "glyph", Page: 1})
	t.Cleanup(func() { _ = m.Close() })
	m.width, m.height, m.kind = 40, 8, "markdown"
	m.markdown = newMarkdownView(&document.Markdown{Source: []byte(text)}, 100)
	m.markdown.raw = true
	m.layoutMarkdown()
	return m
}

func queryDocument(t *testing.T, m *Model, query string) {
	t.Helper()
	m.Update(press("/"))
	if m.search == nil || !m.search.editing {
		t.Fatal("/ did not open search")
	}
	m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	m.Update(tea.PasteMsg{Content: query})
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	// This finite-command helper also drives asynchronous search results;
	// unlike pump it never silently drops work after a timing threshold.
	drainAnimation(t, m, cmd)
}

func TestDocumentSearchInputAndNavigation(t *testing.T) {
	m := searchDocument(t, "ÉCOLE [a].\n\nmiss\n\n\n\n\nécole [a].\nlast")
	queryDocument(t, m, "école [a].")
	if len(m.search.matches) != 2 || m.search.at != 0 {
		t.Fatalf("literal Unicode matches: %+v", m.search)
	}
	if !strings.Contains(m.markdown.view(), "\x1b[30;103m") {
		t.Fatal("match not highlighted")
	}
	m.Update(press("n"))
	if m.search.at != 1 || m.markdown.offset == 0 {
		t.Fatal("next did not scroll")
	}
	m.Update(press("n"))
	if m.search.at != 0 {
		t.Fatal("next did not wrap")
	}
	m.Update(press("N"))
	if m.search.at != 1 {
		t.Fatal("previous did not wrap")
	}
	m.Update(press("/"))
	m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	for _, key := range []string{"q", "o", "?", "/"} {
		m.Update(press(key))
	}
	m.Update(tea.PasteMsg{Content: "https://example.org/ file.png"})
	if m.quitting || m.help || m.screen != screenDocument || string(m.search.draft) != "qo?/https://example.org/ file.png" {
		t.Fatal("query input triggered viewer actions")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.search.editing || m.search.query != "école [a]." {
		t.Fatal("cancel lost committed query")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.search != nil || m.markdown.highlights != nil || m.screen != screenDocument {
		t.Fatal("Esc did not clear only search")
	}
	queryDocument(t, m, "missing")
	if !strings.Contains(m.search.status(80), "no matches") {
		t.Fatal("missing no-match feedback")
	}
	queryDocument(t, m, "")
	if m.search != nil {
		t.Fatal("empty query did not clear")
	}
}

func TestSearchEditorUnicodeAndNarrowViewport(t *testing.T) {
	m := searchDocument(t, "hello")
	m.Update(press("/"))
	m.Update(tea.PasteMsg{Content: "é界🙂"})
	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m.Update(tea.KeyPressMsg{Code: tea.KeyDelete})
	if string(m.search.draft) != "é" || m.search.cursor != 1 {
		t.Fatalf("rune editing: %+v", m.search)
	}
	m.Update(tea.PasteMsg{Content: strings.Repeat("界", 400) + "\x1b\x00"})
	if len(m.search.draft) != maxSearchRunes {
		t.Fatal("query cap not enforced")
	}
	for _, width := range []int{4, 20, 80} {
		line := ansi.Truncate(m.search.status(width), width, "…")
		if ansi.StringWidth(line) > width || !strings.Contains(line, "▏") {
			t.Fatalf("cursor clipped at %d: %q", width, line)
		}
	}
	m.search.draft, m.search.cursor = nil, 0
	m.search.insert("a\n\tb\x00\x1b\u202ec")
	if string(m.search.draft) != "a  bc" {
		t.Fatalf("unsafe query: %q", m.search.draft)
	}
}

func TestSearchReindexesLayoutSourceAndColumns(t *testing.T) {
	m := searchDocument(t, "# Heading\n\nneedle one\n\nneedle two\n")
	queryDocument(t, m, "needle")
	oldRevision := m.search.revision
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 20, Height: 10})
	drainAnimation(t, m, cmd)
	if m.search.revision <= oldRevision || len(m.search.matches) != 2 {
		t.Fatal("resize did not reindex")
	}
	queryDocument(t, m, "# Heading")
	if len(m.search.matches) != 1 {
		t.Fatal("source not searched")
	}
	_, cmd = m.Update(press("s"))
	drainAnimation(t, m, cmd)
	if len(m.search.matches) != 0 {
		t.Fatal("rendered mode searched hidden source markup")
	}

	s := tabulated(t, &document.Sheet{Columns: 2, Rows: [][]string{{"one", "NEEDLE"}, {"needle", "two"}}}, 1, 2)
	s.sheet.setHidden([]bool{false, true})
	queryDocument(t, s, "needle")
	if len(s.search.matches) != 1 || s.sheet.at != [2]int{1, 0} {
		t.Fatal("hidden column was searched, or cell not selected")
	}
	_, cmd = s.Update(press("X"))
	drainAnimation(t, s, cmd)
	if len(s.search.matches) != 2 {
		t.Fatal("shown column not reindexed")
	}
	s.Update(press("N"))
	if s.sheet.at != [2]int{0, 1} {
		t.Fatal("previous did not select matching column")
	}
	if !strings.Contains(s.sheet.view(80, 10), "\x1b[30;103m") {
		t.Fatal("table highlight missing")
	}
	queryDocument(t, s, "two")
	s.Update(document.Result{Generation: s.generation, Kind: "xlsx", Page: 2, Pages: 2, Sheet: &document.Sheet{Columns: 1, Rows: [][]string{{"different"}}}})
	// Above Update scheduled a new snapshot. Drive an explicit replacement
	// to verify old sheet matches cannot survive a page load.
	drainAnimation(t, s, s.startSearch())
	if len(s.search.matches) != 0 {
		t.Fatal("old sheet results retained")
	}
}

func TestSearchStaleResultsAndCancellation(t *testing.T) {
	m := searchDocument(t, strings.Repeat("old\n", 2000))
	m.editSearch()
	m.search.query, m.search.pattern = "old", regexp.MustCompile("old")
	old := m.startSearch()
	results := make(chan tea.Msg, 1)
	go func() { results <- old() }()
	queryDocument(t, m, "absent")
	select {
	case result := <-results:
		m.Update(result)
	case <-time.After(5 * time.Second):
		t.Fatal("canceled search did not finish")
	}
	if m.search.query != "absent" || len(m.search.matches) != 0 {
		t.Fatal("late reply replaced current query")
	}
	late := m.startSearch()()
	m.clearSearch()
	m.Update(late)
	if m.search != nil {
		t.Fatal("late reply reopened cleared search")
	}
	queryDocument(t, m, "old")
	if len(m.search.matches) != maxSearchHits || !strings.Contains(m.search.note, "limit") {
		t.Fatal("match cap not reported")
	}
	m.load(true)
	if m.search != nil {
		t.Fatal("reload did not cancel search")
	}
	m.loading = false
	queryDocument(t, m, "old")
	m.screen = screenList
	m.syncSearch()
	if m.search != nil {
		t.Fatal("leaving document retained search")
	}
}

func TestSearchByteLimitAndUnsupportedMedia(t *testing.T) {
	m := tabulated(t, &document.Sheet{Columns: 1, Rows: [][]string{{strings.Repeat("x", maxSearchBytes) + "needle"}}}, 1, 1)
	queryDocument(t, m, "needle")
	if len(m.search.matches) != 0 || !strings.Contains(m.search.note, "partial") {
		t.Fatal("byte cap not reported")
	}
	m.clearSearch()
	m.sheet, m.kind = nil, "image"
	m.Update(press("/"))
	if m.search != nil {
		t.Fatal("image offered text search")
	}
	m.kind = "stl"
	m.Update(press("/"))
	if m.search != nil {
		t.Fatal("mesh offered text search")
	}
}

func TestPDFSearchNavigatesToTextPage(t *testing.T) {
	m := New(Options{Files: []string{"../../examples/field-guide.pdf"}, Render: "glyph", Page: 1})
	t.Cleanup(func() { _ = m.Close() })
	m.kind, m.pages, m.width, m.height = "pdf", 2, 80, 24
	m.editSearch()
	m.search.query, m.search.pattern = "small files", regexp.MustCompile("(?i)small files")
	result := m.startSearch()().(searchResult)
	if result.note != "" || len(result.matches) != 1 || result.matches[0].page != 2 || !strings.Contains(result.matches[0].excerpt, "SMALL FILES") {
		t.Fatalf("PDF text search: %+v", result)
	}
	cmd := m.searched(result)
	if cmd == nil || m.page != 2 || !m.loading {
		t.Fatal("PDF match did not request page 2")
	}
	if m.search.unit != "pages" {
		t.Fatal("PDF count must name pages, not occurrences")
	}
	m.clearSearch()
	m.Update(result)
	if m.search != nil {
		t.Fatal("PDF reply revived cleared search")
	}
}

func TestPDFSearchReportsSkippedAndUnreadablePages(t *testing.T) {
	// An original one-page PDF with no contents stream, like an empty scan.
	var pdf strings.Builder
	pdf.WriteString("%PDF-1.4\n")
	var offsets []int
	for i, body := range []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>",
	} {
		offsets = append(offsets, pdf.Len())
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", i+1, body)
	}
	start := pdf.Len()
	pdf.WriteString("xref\n0 4\n0000000000 65535 f \n")
	for _, offset := range offsets {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", start)
	for _, tc := range []struct{ data, want string }{
		{pdf.String(), "1 pages without text skipped"},
		{"%PDF-1.4\nbroken", "partial results: page 1:"},
	} {
		m := New(Options{Files: []string{"test.pdf"}, FilesFS: fstest.MapFS{"test.pdf": &fstest.MapFile{Data: []byte(tc.data)}}, Render: "glyph", Page: 1})
		m.kind, m.pages = "pdf", 1
		m.editSearch()
		m.search.query, m.search.pattern = "needle", regexp.MustCompile("needle")
		result := m.startSearch()().(searchResult)
		if len(result.matches) != 0 || !strings.Contains(result.note, tc.want) {
			t.Fatalf("PDF report: %+v", result)
		}
		_ = m.Close()
	}
}

func TestHighlightSearchPreservesANSIAndUnicode(t *testing.T) {
	for _, text := range []string{
		"é界e\u0301🙂 hello",
		"\x1b[31mhe\x1b[0mllo\x1b[34m end\x1b[0m",
		ansi.SetHyperlink("https://example.org") + "hello" + ansi.ResetHyperlink(),
	} {
		query := "hello"
		if strings.HasPrefix(text, "é") {
			query = "界e\u0301🙂"
		}
		got := highlightSearch(text, regexp.MustCompile(regexp.QuoteMeta(query)))
		if ansi.Strip(got) != ansi.Strip(text) || ansi.StringWidth(got) != ansi.StringWidth(text) {
			t.Fatalf("highlight corrupted content: %q", got)
		}
		if !strings.Contains(got, "\x1b[30;103m") {
			t.Fatalf("no highlight: %q", got)
		}
		if strings.Contains(text, "https://") && !strings.Contains(got, ansi.SetHyperlink("https://example.org")) {
			t.Fatal("OSC link lost")
		}
	}
	got := highlightSearch("\x1b[31mhello world\x1b[0m", regexp.MustCompile("hello"))
	if !strings.Contains(got, "\x1b[0m\x1b[31m world") {
		t.Fatalf("original style not restored: %q", got)
	}
}
