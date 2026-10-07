package app

import (
	"fmt"
	"image"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/charmbracelet/x/ansi"
)

func TestSearchWrapIndependentMatchesAndSelection(t *testing.T) {
	for _, tc := range []struct {
		name, source, query string
		raw                 bool
		count               int
	}{
		{"source", "start needle phrase and needle phrase end", "needle phrase", true, 2},
		{"paragraph", "start **needle** phrase and needle *phrase* end", "needle phrase", false, 2},
		{"code", "```go\nvar a = \"needle phrase\"\nvar b = \"needle phrase\"\n```", "needle phrase", false, 2},
		{"links", "[needle phrase](https://example.org/long-address) and needle phrase", "needle phrase", false, 2},
		{"lists", "> - start needle phrase and more text\n> - needle phrase again", "needle phrase", false, 2},
		{"unicode", "é界é🙂needlephrase e界é🙂needlephrase", "界é🙂needlephrase", false, 2},
		{"table", "| Name | Value |\n| :--- | ---: |\n| needle phrase | unrelated other text |\n| another | needle phrase |", "needle phrase", false, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := searchDocument(t, tc.source)
			m.markdown.raw = tc.raw
			m.layoutMarkdown()
			queryDocument(t, m, tc.query)
			if len(m.search.matches) != tc.count {
				t.Fatalf("initial results: %+v", m.search.matches)
			}
			m.Update(press("n"))
			selected := m.search.matches[m.search.at]
			content := m.markdown.content
			for _, width := range []int{80, 17, 7, 3, 1, 40} {
				_, cmd := m.Update(tea.WindowSizeMsg{Width: width, Height: 15})
				drainAnimation(t, m, cmd)
				if len(m.search.matches) != tc.count || m.search.at != 1 {
					t.Fatalf("width %d: count/selection changed: %+v", width, m.search)
				}
				hit := m.search.matches[m.search.at]
				if hit.unit != selected.unit || hit.start != selected.start || m.markdown.content != content {
					t.Fatalf("width %d: logical anchor changed", width)
				}
				assertTextSpans(t, m.markdown, width)
				fragments := 0
				for _, spans := range m.markdown.highlights {
					fragments += len(spans)
				}
				if width >= 7 && fragments < 2 {
					t.Fatalf("width %d: wrapped fragments not highlighted: %+v", width, m.markdown.highlights)
				}
			}
		})
	}
}

func assertTextSpans(t *testing.T, md *markdownView, width int) {
	t.Helper()
	for row, line := range md.lines {
		if line.image >= 0 {
			continue
		}
		if ansi.StringWidth(line.text) > width {
			t.Fatalf("row %d exceeds width %d: %q", row, width, line.text)
		}
		plain := ansi.Strip(line.text)
		for _, span := range line.spans {
			unit := md.content.units[span.unit].plain
			if span.start < 0 || span.end > len(unit) || span.offset+span.end-span.start > len(plain) {
				t.Fatalf("bad span: %+v in %q", span, plain)
			}
			if got, want := plain[span.offset:span.offset+span.end-span.start], unit[span.start:span.end]; got != want {
				t.Fatalf("row %d: mapped %q, want %q (span %+v)", row, got, want, span)
			}
		}
		for _, hit := range md.highlights[row] {
			if hit[0] < 0 || hit[1] > len(plain) || hit[0] >= hit[1] {
				t.Fatalf("bad highlight: %v in %q", hit, plain)
			}
		}
		if ansi.Strip(highlightRanges(line.text, md.highlights[row])) != plain {
			t.Fatal("highlight changed displayed text")
		}
	}
}

func TestSearchDoesNotJoinHardLinesParagraphsOrTableCells(t *testing.T) {
	for i, source := range []string{"left\nright", "left\n\nright", "| A | B |\n| --- | --- |\n| left | right |"} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			m := searchDocument(t, source)
			m.markdown.raw = i == 0
			m.layoutMarkdown()
			for _, width := range []int{80, 4} {
				m.width = width
				m.layoutMarkdown()
				queryDocument(t, m, "left right")
				if len(m.search.matches) != 0 {
					t.Fatal("unrelated logical text was joined")
				}
			}
		})
	}
}

func TestMarkdownReflowImagesAndTableCellLinks(t *testing.T) {
	source := "before\n\n| Photo | Link |\n| --- | --- |\n| ![needle phrase](local.png) | [needle phrase](https://example.org/) |\n\nafter"
	m := searchDocument(t, source)
	m.markdown = newMarkdownView(&document.Markdown{Source: []byte(source), Images: []document.MarkdownImage{{Alt: "needle phrase", Image: image.NewRGBA(image.Rect(0, 0, 4, 4))}}}, 100)
	m.layoutMarkdown()
	queryDocument(t, m, "needle phrase")
	if len(m.search.matches) != 2 {
		t.Fatalf("image label/link results: %+v", m.search.matches)
	}
	for _, width := range []int{40, 12, 4} {
		_, cmd := m.Update(tea.WindowSizeMsg{Width: width, Height: 25})
		drainAnimation(t, m, cmd)
		assertTextSpans(t, m.markdown, width)
		images, links := 0, false
		for row, line := range m.markdown.lines {
			if line.image >= 0 {
				images++
				if len(m.markdown.highlights[row]) != 0 {
					t.Fatal("image pixels highlighted")
				}
			}
			if strings.Contains(line.text, "https://example.org/") {
				links = true
			}
			if strings.ContainsRune(line.text, 0xe000) {
				t.Fatal("internal marker leaked")
			}
		}
		if images == 0 || !links {
			t.Fatalf("width %d lost image or OSC hyperlink", width)
		}
	}
}

func TestSearchReflowDiscardsOldProjection(t *testing.T) {
	m := searchDocument(t, "needle phrase then needle phrase")
	queryDocument(t, m, "needle phrase")
	m.Update(press("n"))
	selected := m.search.matches[1]
	m.width = 7
	m.layoutMarkdown()
	old := m.syncSearch()
	m.width = 4
	m.layoutMarkdown()
	current := m.syncSearch()
	m.Update(old())
	if !m.search.running || m.markdown.highlights != nil {
		t.Fatal("obsolete layout supplied highlights")
	}
	m.Update(current())
	if m.search.at != 1 || m.search.matches[1].start != selected.start {
		t.Fatal("resize burst lost logical selection")
	}
	assertTextSpans(t, m.markdown, 4)
}

func TestSearchBoundsOccurrencesWithinOneLogicalLine(t *testing.T) {
	m := searchDocument(t, strings.Repeat("needle ", maxSearchHits+1))
	queryDocument(t, m, "needle")
	if len(m.search.matches) != maxSearchHits || !strings.Contains(m.search.note, "partial") {
		t.Fatal("single-line occurrence cap not enforced")
	}
}
