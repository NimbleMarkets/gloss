package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/NimbleMarkets/gloss/internal/document"
)

// pageRange is what --page asks for: one page, some, or all. The viewer
// takes one; an export or a text into a folder may take a range.
type pageRange struct {
	pages []int // Counted from 1, in the order asked; empty for all.
	all   bool
}

// parsePages reads a page number, numbers and ranges such as 1,3-5, or all.
func parsePages(s string) (pageRange, error) {
	s = strings.TrimSpace(s)
	if strings.EqualFold(s, "all") {
		return pageRange{all: true}, nil
	}
	var r pageRange
	for _, part := range strings.Split(s, ",") {
		first, last, found := strings.Cut(strings.TrimSpace(part), "-")
		from, err := strconv.Atoi(first)
		to := from
		if err == nil && found {
			to, err = strconv.Atoi(last)
		}
		if err != nil || from < 1 || to < from {
			return r, fmt.Errorf("--page: %q is not a page number, a range like 2-5, or all", part)
		}
		// No document has more pages than that, and a range is listed in
		// full: 1-9999999999 must not be given the memory to be.
		if to-from >= document.MaxPages || len(r.pages)+(to-from+1) > document.MaxPages {
			return r, fmt.Errorf("--page: %q names more than %d pages; use all for every page", part, document.MaxPages)
		}
		for i := 0; i <= to-from; i++ {
			r.pages = append(r.pages, from+i)
		}
	}
	if len(r.pages) == 0 {
		return r, fmt.Errorf("--page names no page")
	}
	return r, nil
}

// single says whether one page was asked for, and which.
func (r pageRange) single() (int, bool) {
	if r.all || len(r.pages) != 1 {
		return 0, false
	}
	return r.pages[0], true
}

// of lists the pages asked for, of a document with n pages.
func (r pageRange) of(n int) []int {
	if r.all {
		pages := make([]int, 0, n)
		for i := 1; i <= n; i++ {
			pages = append(pages, i)
		}
		return pages
	}
	var pages []int
	for _, p := range r.pages {
		if p <= n {
			pages = append(pages, p)
		}
	}
	return pages
}
