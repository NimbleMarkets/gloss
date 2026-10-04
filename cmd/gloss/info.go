package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/NimbleMarkets/ntcharts-svg/svg"
)

// described is what a file says about itself, for a program to read.
type described struct {
	Path    string                       `json:"path" doc:"The file, as named; stdin for -."`
	Kind    string                       `json:"kind,omitempty" doc:"The format gloss read it as."`
	Page    int                          `json:"page,omitempty" doc:"The page described, of a PDF."` // Of a PDF.
	Pages   int                          `json:"pages,omitempty" doc:"How many pages a PDF has."`
	Details map[string]map[string]string `json:"details,omitempty" doc:"What the viewer's i box shows, by section, then by label: details.Model.Triangles."` // By section, then by label.
	Error   string                       `json:"error,omitempty" doc:"Why this file could not be described; the others still are."`

	fields []document.Field // In the order of the viewer's details box.
}

// describe prints what each file says about itself: what the viewer's
// details box shows, without the viewer. A file that cannot be described is
// reported among the others, and makes the exit status 1.
func describe(opts options, stdout, stderr io.Writer) error {
	if len(opts.Files) == 0 {
		return fmt.Errorf("--info needs a file to describe")
	}
	loader := &document.Loader{}
	defer loader.Close()
	files, failed := make([]described, 0, len(opts.Files)), 0
	for i, path := range opts.Files {
		d := described{Path: path}
		if opts.IsStdin(path) {
			d.Path = "stdin"
		}
		if err := opts.Accept.CheckFile(nil, path); err != nil {
			d.Error = document.SkipReason(err)
		} else if kind, err := document.Probe(path, opts.Type); err != nil {
			d.Error = document.SkipReason(err)
		} else {
			r := loader.Load(document.Request{Path: path, Type: opts.Type, Page: opts.Page, DPI: opts.DPI, Generation: uint64(i + 1), Parts: opts.Parts, Stdin: opts.IsStdin(path)})
			if d.Kind, d.fields = r.Kind, r.Info; d.Kind == "" {
				d.Kind = kind
			}
			if r.Kind == "pdf" && r.Err == nil {
				d.Page, d.Pages = r.Page, r.Pages
			}
			if r.Err != nil {
				d.Error = r.Err.Error()
			}
		}
		section := ""
		for _, f := range d.fields {
			if f.Value == "" {
				section = f.Label
				continue
			}
			if d.Details == nil {
				d.Details = map[string]map[string]string{}
			}
			if d.Details[section] == nil {
				d.Details[section] = map[string]string{}
			}
			d.Details[section][f.Label] = f.Value
		}
		if d.Error != "" {
			failed++
		}
		files = append(files, d)
	}
	if opts.JSON {
		out := json.NewEncoder(stdout)
		out.SetIndent("", "  ")
		out.SetEscapeHTML(false)
		if err := out.Encode(files); err != nil {
			return err
		}
	} else {
		for i, d := range files {
			if i > 0 {
				fmt.Fprintln(stdout)
			}
			fmt.Fprintln(stdout, svg.SanitizeForTerminal(d.Path))
			width := 0
			for _, f := range d.fields {
				if f.Value != "" {
					width = max(width, len([]rune(f.Label)))
				}
			}
			for _, f := range d.fields {
				if f.Value == "" {
					fmt.Fprintf(stdout, "  %s\n", svg.SanitizeForTerminal(f.Label))
					continue
				}
				fmt.Fprintf(stdout, "    %s%s  %s\n", svg.SanitizeForTerminal(f.Label), strings.Repeat(" ", width-len([]rune(f.Label))), svg.SanitizeForTerminal(f.Value))
			}
			if d.Error != "" {
				fmt.Fprintf(stdout, "  Cannot open: %s\n", svg.SanitizeForTerminal(d.Error))
			}
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d files could not be described", failed, len(files))
	}
	return nil
}
