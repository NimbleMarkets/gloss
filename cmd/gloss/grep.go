package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/NimbleMarkets/ntcharts-svg/svg"
)

const (
	maxGrepHits    = 200 // Matches reported in one run; past this, gloss stops and says so.
	excerptContext = 60  // Runes of context on each side of a match.
	excerptMatch   = 120 // Runes of a long match kept in its excerpt.
)

// grepFiles searches the text layer of PDF pages, every page unless --page
// says which, and reports each match: its page and a line of context around
// it, with --json as one object each. It does not print the text itself.
// Pages with no text layer cannot be searched, and stderr names them; an
// input that is not a PDF is an error among the others.
func grepFiles(opts options, stdout, stderr io.Writer) error {
	loader := &document.Loader{}
	defer loader.Close()
	hits := []made{}
	var failed error
	generation, truncated := 0, false
	fail := func(entry made, path string, err error) {
		entry.Error = err.Error()
		hits = append(hits, entry)
		fmt.Fprintf(stderr, "gloss: %s: %s\n", svg.SanitizeForTerminal(path), svg.SanitizeForTerminal(err.Error()))
		if failed == nil {
			failed = fmt.Errorf("%s: %w", path, err)
		}
	}
	for _, path := range opts.Files {
		if truncated {
			break
		}
		name := path
		if opts.IsStdin(path) {
			name = "stdin"
		}
		var unsearched []string
	pages:
		for _, page := range pagesOf(loader, opts, path, &generation) {
			generation++
			q := exportRequest(opts.Options, path, generation)
			q.Page, q.TextOnly = page, true
			r := loader.Load(q)
			entry := made{Path: name, Kind: r.Kind, Page: r.Page, Pages: r.Pages}
			switch {
			case errors.Is(r.Err, document.ErrNoTextLayer):
				unsearched = append(unsearched, fmt.Sprint(r.Page))
				continue
			case r.Err != nil:
				fail(entry, path, r.Err)
				continue
			case r.Kind != "pdf":
				err := fmt.Errorf("--grep searches the text layer of a PDF; for this %s file, use --text", r.Kind)
				if _, _, textErr := document.Text(r); textErr != nil {
					err = textErr // A picture: it says to use --output.
				}
				entry.Page, entry.Pages = 0, 0
				fail(entry, path, err)
				break pages // One error for the file, not one per sheet.
			}
			for _, at := range opts.Grep.FindAllStringIndex(r.Text, -1) {
				if len(hits) >= maxGrepHits {
					truncated = true
					break pages
				}
				entry.Excerpt = excerpt(r.Text, at[0], at[1])
				hits = append(hits, entry)
				if !opts.JSON {
					prefix := ""
					if len(opts.Files) > 1 {
						prefix = svg.SanitizeForTerminal(name) + ":"
					}
					fmt.Fprintf(stdout, "%s%d: %s\n", prefix, entry.Page, entry.Excerpt)
				}
			}
		}
		if len(unsearched) > 0 {
			fmt.Fprintf(stderr, "gloss: %s: %s %s %s no text layer and %s not searched; export with --output to see %s\n",
				svg.SanitizeForTerminal(path), plural(len(unsearched), "page"), strings.Join(unsearched, ", "),
				map[bool]string{true: "has", false: "have"}[len(unsearched) == 1], map[bool]string{true: "was", false: "were"}[len(unsearched) == 1],
				map[bool]string{true: "it", false: "them"}[len(unsearched) == 1])
		}
	}
	if truncated {
		note := fmt.Sprintf("stopped at %d matches: narrow the pattern or --page", maxGrepHits)
		hits[len(hits)-1].Note = note
		fmt.Fprintln(stderr, "gloss: "+note)
	}
	if opts.JSON {
		if err := json.NewEncoder(stdout).Encode(hits); err != nil {
			return err
		}
	}
	return once(failed)
}

// excerpt is the match from start to end in text, with some words either
// side, on one line: whitespace runs become one space, and control
// characters, which a terminal would act on, are dropped. A cut is marked.
func excerpt(text string, start, end int) string {
	from, before := start, 0
	for from > 0 && before < excerptContext {
		_, size := utf8.DecodeLastRuneInString(text[:from])
		from -= size
		before++
	}
	to, matched := start, 0
	for to < end && matched < excerptMatch {
		_, size := utf8.DecodeRuneInString(text[to:])
		to += size
		matched++
	}
	cut := to < end
	for after := 0; to < len(text) && after < excerptContext; after++ {
		_, size := utf8.DecodeRuneInString(text[to:])
		to += size
	}
	var b strings.Builder
	if from > 0 {
		b.WriteString("…")
	}
	space := false
	for _, r := range text[from:to] {
		switch {
		case unicode.IsSpace(r):
			space = b.Len() > 0
			continue
		case unicode.IsControl(r):
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(r)
	}
	if to < len(text) || cut {
		b.WriteString("…")
	}
	return strings.TrimSpace(b.String())
}
