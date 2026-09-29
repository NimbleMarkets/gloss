// gloss is a terminal pager for images, SVGs, PDFs, and STL meshes.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/app"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/pflag"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "gloss: %v\n", err)
		os.Exit(1)
	}
}

func parse(args []string, out io.Writer) (app.Options, bool, error) {
	var opts app.Options
	f := pflag.NewFlagSet("gloss", pflag.ContinueOnError)
	f.SetOutput(out)
	f.StringVarP(&opts.Render, "render", "r", "auto", "terminal graphics: auto, kitty, glyph")
	f.StringVar(&opts.Render3D, "3d", "auto", "STL renderer: auto, software, wireframe")
	f.StringVarP(&opts.Type, "type", "t", "", "force input type: image, svg, pdf, stl, markdown (or md)")
	f.IntVarP(&opts.Page, "page", "p", 1, "initial PDF page (1-based)")
	f.IntVarP(&opts.DPI, "dpi", "d", 150, "PDF rasterization DPI (36–600)")
	f.BoolVarP(&opts.Menu, "menu", "m", false, "start with the file-selection menu")
	f.BoolVarP(&opts.Preview, "preview", "P", false, "start with the file menu and a preview pane")
	f.StringVarP(&opts.Output, "output", "o", "", "export one input as PNG; '-' writes PNG to stdout")
	f.StringVarP(&opts.OutputDir, "output-dir", "O", "", "export each input as a numbered PNG in this directory")
	f.IntVarP(&opts.MaxEdge, "max-edge", "s", 1536, "maximum exported image edge in pixels (1–4096)")
	f.StringVar(&opts.VisionProfile, "vision-profile", "", "export sizing: openai-high, claude-standard, claude-high")
	showVersion := f.BoolP("version", "V", false, "print version")
	showHelp := f.BoolP("help", "h", false, "show help")
	f.Usage = func() {
		fmt.Fprint(out, "Usage: gloss [options] [file | folder]...\n       command | gloss [options] -\n\nA visual pager for images, SVG, PDF, STL and Markdown.\nWith no file, gloss opens empty: drop files on it, or press o to browse.\nA folder opens the file browser there.\nOptions may appear before or after filenames. Use -- to end options.\n\n")
		f.PrintDefaults()
		fmt.Fprintln(out, "\nKeys: q quit · ? help · [/] files · n/p pages · +/- zoom · arrows pan/orbit")
	}
	if err := f.Parse(args); err != nil {
		return opts, errors.Is(err, pflag.ErrHelp), err
	}
	if *showHelp {
		f.Usage()
		return opts, true, nil
	}
	if *showVersion {
		fmt.Fprintln(out, "gloss "+version)
		return opts, true, nil
	}
	if !slices.Contains([]string{"auto", "kitty", "glyph"}, opts.Render) {
		return opts, false, fmt.Errorf("--render must be auto, kitty, or glyph")
	}
	if !slices.Contains([]string{"auto", "software", "wireframe"}, opts.Render3D) {
		return opts, false, fmt.Errorf("--3d must be auto, software, or wireframe")
	}
	if opts.Type == "md" {
		opts.Type = "markdown"
	}
	if !slices.Contains([]string{"", "image", "svg", "pdf", "stl", "markdown"}, opts.Type) {
		return opts, false, fmt.Errorf("--type must be image, svg, pdf, stl, or markdown")
	}
	if opts.Page < 1 {
		return opts, false, fmt.Errorf("--page must be at least 1")
	}
	if opts.DPI < 36 || opts.DPI > 600 {
		return opts, false, fmt.Errorf("--dpi must be between 36 and 600")
	}
	opts.Files = f.Args()
	if opts.VisionProfile != "" {
		profile, ok := document.VisionProfiles[opts.VisionProfile]
		if !ok {
			return opts, false, fmt.Errorf("unknown vision profile %q", opts.VisionProfile)
		}
		if !f.Changed("max-edge") {
			opts.MaxEdge = profile.MaxEdge
		}
		if opts.Output == "" && opts.OutputDir == "" {
			return opts, false, fmt.Errorf("--vision-profile requires --output or --output-dir")
		}
	}
	if opts.MaxEdge < 1 || opts.MaxEdge > 4096 {
		return opts, false, fmt.Errorf("--max-edge must be 1..4096")
	}
	if opts.Output != "" && opts.OutputDir != "" {
		return opts, false, fmt.Errorf("choose --output or --output-dir")
	}
	if opts.Output != "" && len(opts.Files) > 1 {
		return opts, false, fmt.Errorf("--output accepts one input; use --output-dir for multiple inputs")
	}
	if (opts.Output != "" || opts.OutputDir != "") && (opts.Menu || opts.Preview) {
		return opts, false, fmt.Errorf("export cannot be combined with --menu or --preview")
	}
	return opts, false, nil
}

func run(args []string) error {
	opts, done, err := parse(args, os.Stdout)
	if done {
		return nil
	}
	if err != nil {
		return err
	}
	stdinTTY := term.IsTerminal(os.Stdin.Fd())
	opts.MarkdownBase, err = os.Getwd()
	if err != nil {
		return err
	}
	if len(opts.Files) == 0 && !stdinTTY {
		opts.Files = []string{"-"}
	}
	exporting := opts.Output != "" || opts.OutputDir != ""
	if opts.Output == "-" && term.IsTerminal(os.Stdout.Fd()) {
		return fmt.Errorf("redirect PNG stdout to a file or pipe")
	}
	if !exporting && !term.IsTerminal(os.Stdout.Fd()) {
		return fmt.Errorf("output must be a terminal; run gloss directly in your terminal, or see gloss --help")
	}
	stdinPath := ""
	defer func() {
		if stdinPath != "" {
			_ = os.Remove(stdinPath)
		}
	}()
	for i, path := range opts.Files {
		if path == "-" {
			if stdinTTY {
				return fmt.Errorf("'-' requires piped or redirected input")
			}
			if stdinPath == "" {
				data, err := document.ReadLimited(os.Stdin)
				if err != nil {
					return fmt.Errorf("stdin: %w", err)
				}
				f, err := os.CreateTemp("", "gloss-stdin-*")
				if err != nil {
					return err
				}
				stdinPath = f.Name()
				_, writeErr := f.Write(data)
				closeErr := f.Close()
				if err := errors.Join(writeErr, closeErr); err != nil {
					return err
				}
			}
			opts.Files[i] = stdinPath
			continue
		}
	}
	opts.Browse, opts.Files, err = inputs(opts.Files, opts.Type, exporting, os.Stderr)
	if err != nil {
		return err
	}

	if exporting {
		return exportFiles(opts, os.Stdout, os.Stderr)
	}
	m := app.New(opts)
	defer m.Close()
	// Bubble Tea opens the controlling TTY automatically when stdin is a pipe.
	_, err = tea.NewProgram(m).Run()
	// Dropped files are reported once the alternate screen is gone.
	for _, line := range m.Skipped() {
		fmt.Fprintln(os.Stderr, line)
	}
	if err != nil {
		return err
	}
	return m.Err()
}

var errNoInputs = errors.New("no supported input files")

// inputs sorts the arguments into files to show and a folder to browse.
// Given files, folders are skipped, as a glob needs. A folder is browsed only
// when there is nothing else to show, and with no arguments at all the viewer
// opens empty.
func inputs(paths []string, forced string, exporting bool, stderr io.Writer) (string, []string, error) {
	if exporting {
		if len(paths) == 0 {
			return "", nil, fmt.Errorf("no input: name a file to export")
		}
		files, err := supportedFiles(paths, forced, stderr)
		return "", files, err
	}
	if len(paths) == 0 {
		return "", nil, nil
	}
	var skipped bytes.Buffer
	files, err := supportedFiles(paths, forced, &skipped)
	if errors.Is(err, errNoInputs) {
		for i, path := range paths {
			if info, statErr := os.Stat(path); statErr == nil && info.IsDir() {
				// Report the rest, but not the folder that is put to use.
				if rest := slices.Delete(slices.Clone(paths), i, i+1); len(rest) > 0 {
					_, _ = supportedFiles(rest, forced, stderr)
				}
				return path, nil, nil
			}
		}
	}
	_, _ = stderr.Write(skipped.Bytes())
	return "", files, err
}

// Unsupported arguments are skipped so shell globs can contain unrelated files.
func supportedFiles(paths []string, forced string, stderr io.Writer) ([]string, error) {
	files := make([]string, 0, len(paths))
	for _, path := range paths {
		_, err := document.Probe(path, forced)
		if errors.Is(err, document.ErrUnsupported) || errors.Is(err, document.ErrNotRegular) || errors.Is(err, document.ErrDirectory) {
			fmt.Fprintln(stderr, document.Skipped(path, err))
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%q: %w", path, err)
		}
		files = append(files, path)
	}
	if len(files) == 0 {
		return nil, errNoInputs
	}
	return files, nil
}
