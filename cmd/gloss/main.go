// gloss is a terminal pager for images, SVGs, PDFs, STL and 3MF meshes, and
// Markdown, Word, and Excel files.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/NimbleMarkets/gloss/internal/app"
	"github.com/NimbleMarkets/gloss/internal/document"
	"github.com/NimbleMarkets/gloss/skills"
	charts "github.com/NimbleMarkets/ntcharts3d"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/pflag"
)

var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "gloss: %v\n", err)
		os.Exit(exitCode(err))
	}
}

// options are the viewer's, and those of showing it somewhere else.
type options struct {
	app.Options
	Serve, NoOpen bool
	Timeout       time.Duration
	Info, JSON    bool
	FetchAllowed  bool
	Globs         []string
	Text          bool      // Take the text out, rather than draw.
	Pages         pageRange // What --page asked for; Page holds it when it is one.
	Resume        string    // Token of a detached pick whose answer is wanted.
	Status        string    // Token of a detached pick whose state is wanted, now.
	Detached      string    // Set on the server a detached start runs: its token.
	VisionProfile string    // A convenience alias for a max edge; --max-edge is the stable flag.
	EdgeReason    string    // Why the alias chose its edge, for the manifest.
	SkillInstall  string    // Where --skill --install writes, "auto" to find the agents here.
}

func parse(args []string, out io.Writer) (options, bool, error) {
	var opts options
	f := pflag.NewFlagSet("gloss", pflag.ContinueOnError)
	f.SetOutput(out)
	f.StringVarP(&opts.Render, "render", "r", "auto", "terminal graphics: auto, kitty, glyph")
	f.StringVar(&opts.Render3D, "3d", "auto", "mesh renderer for STL and 3MF: auto, software, wireframe")
	f.StringVarP(&opts.Type, "type", "t", "", "force input type: image, svg, pdf, stl, 3mf, docx, xlsx, grist, csv, json, ipynb, html, text, markdown (or md)")
	page := f.StringP("page", "p", "1", "PDF page or workbook sheet, from 1; for an export or text into a folder, a range such as 2-5, or all")
	f.BoolVar(&opts.Text, "text", false, "take the text out: Markdown of a Word document, page, or notebook, text of a PDF page, CSV of a sheet, text and JSON as they are")
	f.IntVarP(&opts.DPI, "dpi", "d", 150, "PDF rasterization DPI (36–600)")
	f.BoolVarP(&opts.Menu, "menu", "m", false, "start with the file-selection menu")
	f.BoolVarP(&opts.Preview, "preview", "P", false, "start with the file menu and a preview pane")
	view := f.String("view", "", "views of a mesh: front, back, left, right, top, bottom, iso, all; several, as front,top, export as one sheet")
	camera := f.String("camera", "", "camera for a mesh, as elevation,azimuth or elevation,azimuth,distance in degrees")
	projection := f.String("projection", "ortho", "projection of a mesh: ortho, perspective")
	f.StringVar(&opts.Prompt, "prompt", "", "show this request to the user in a box, to say what to pick or look at")
	promptLoc := f.String("prompt-loc", "bottom", "where the terminal prompt box goes: bottom or top (browser requests always appear above the viewer)")
	paint := f.String("color", "", "paint the faces of a mesh its file left plain: #rrggbb or a name such as orange")
	parts := f.String("parts", "", "show only these parts of a 3MF, by name: name,name")
	partn := f.String("partn", "", "show only these parts of a 3MF, counted from 1: 2,4-6")
	cols := f.String("cols", "", "show only these columns of a table, by header or letter: name,name")
	coln := f.String("coln", "", "show only these columns of a table, counted from 1: 2,4-6")
	f.StringArrayVar(&opts.Globs, "glob", nil, "search the folders named, or the current one, for files: a glob, an extension, or a kind such as images (repeatable)")
	f.BoolVar(&opts.FetchAllowed, "fetch", false, "allow opening web addresses found in tables, with Enter; gloss never fetches on its own")
	f.BoolVar(&opts.Info, "info", false, "print what each file says about itself, and do not open the viewer")
	f.BoolVar(&opts.JSON, "json", false, "with --info, print JSON")
	f.BoolVar(&opts.Pick, "pick", false, "wait for the user to hand over files: Enter prints their paths and quits")
	f.BoolVar(&opts.Serve, "serve", false, "show the viewer on a web page, from a temporary server on this machine")
	f.BoolVar(&opts.NoOpen, "no-open", false, "with --serve, print the page's address without opening a browser")
	f.DurationVar(&opts.Timeout, "timeout", 0, "with --serve or --pick, give up after this long (as 90s or 10m)")
	f.BoolVarP(&opts.KeepScreen, "no-alt-screen", "X", false, "draw on the main screen: scrollback is kept, and the last view stays after quitting")
	f.StringVarP(&opts.Output, "output", "o", "", "export one input as PNG; '-' writes PNG to stdout")
	f.StringVarP(&opts.OutputDir, "output-dir", "O", "", "export each input as a numbered PNG in this directory")
	f.IntVarP(&opts.MaxEdge, "max-edge", "s", 1536, "maximum exported image edge in pixels (1–4096)")
	f.StringVar(&opts.VisionProfile, "vision-profile", "", "alias for a --max-edge, which is stable where these go stale: "+strings.Join(document.VisionProfileNames(), ", "))
	f.StringVar(&opts.Resume, "resume", "", "print the answer of a pick that was started without a terminal, by the token it printed; with --timeout, stop waiting after that long")
	f.StringVar(&opts.Status, "status", "", "print, as JSON and at once, how a pick started without a terminal stands: waiting, picked, declined, timeout, closed, or failed; it waits for nothing and changes nothing")
	f.StringVar(&opts.Detached, "detached", "", "")
	_ = f.MarkHidden("detached")
	showVersion := f.BoolP("version", "V", false, "print version")
	showSkill := f.Bool("skill", false, "print the skill that teaches agents to use gloss (SKILL.md), and exit")
	skillInstall := f.String("install", "", "with --skill, install the skill for the agents found on this machine, or into the skills folder named (--install=FOLDER)")
	f.Lookup("install").NoOptDefVal = "auto"
	showHelp := f.BoolP("help", "h", false, "show help")
	// The reference is written from these options; see docs.go.
	docsMan := f.String("docs-man", "", "")
	docsMarkdown := f.String("docs-markdown", "", "")
	docsHugo := f.Bool("docs-hugo", false, "")
	docsCompletions := f.String("docs-completions", "", "")
	for _, name := range []string{"docs-man", "docs-markdown", "docs-hugo", "docs-completions"} {
		_ = f.MarkHidden(name)
	}
	f.Usage = func() {
		fmt.Fprint(out, "Usage: "+usageLines[0]+"\n       "+usageLines[1]+"\n\n"+usageAbout+"\n\n")
		f.PrintDefaults()
		fmt.Fprintln(out, "\nKeys: q quit · ? help · [/] files · n/p pages · +/- zoom · arrows pan/orbit")
		fmt.Fprintln(out, "Documentation: "+app.DocsURL)
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
	if *docsMan != "" || *docsMarkdown != "" || *docsCompletions != "" {
		var wrote bytes.Buffer
		err := writeDocs(f, docsRequest{ManDir: *docsMan, MarkdownDir: *docsMarkdown, CompletionsDir: *docsCompletions, Hugo: *docsHugo}, &wrote)
		out.Write(wrote.Bytes())
		return opts, err == nil, err
	}
	if *showSkill {
		if f.Changed("install") {
			opts.SkillInstall = *skillInstall
			return opts, false, nil
		}
		fmt.Fprint(out, skills.Gloss)
		return opts, true, nil
	}
	if f.Changed("install") {
		return opts, false, fmt.Errorf("--install goes with --skill: gloss --skill --install")
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
	if !slices.Contains([]string{"", "image", "svg", "pdf", "stl", "3mf", "docx", "xlsx", "grist", "csv", "json", "ipynb", "html", "text", "markdown"}, opts.Type) {
		return opts, false, fmt.Errorf("--type must be image, svg, pdf, stl, 3mf, docx, xlsx, grist, csv, json, ipynb, html, text, or markdown")
	}
	pages, err := parsePages(*page)
	if err != nil {
		return opts, false, err
	}
	opts.Pages = pages
	if single, ok := opts.Pages.single(); ok {
		opts.Page = single
	} else if opts.OutputDir == "" && !opts.JSON {
		return opts, false, fmt.Errorf("--page with a range or all needs --output-dir, or --json with --text")
	}
	if opts.DPI < 36 || opts.DPI > 600 {
		return opts, false, fmt.Errorf("--dpi must be between 36 and 600")
	}
	opts.Files = f.Args()
	if opts.Columns, err = document.ParseColumns(*cols, *coln); err != nil {
		return opts, false, err
	}
	if opts.Parts, err = document.ParseParts(*parts, *partn); err != nil {
		return opts, false, err
	}
	if *paint != "" {
		c, err := document.ParseColor(*paint)
		if err != nil {
			return opts, false, err
		}
		opts.Color = &c
	}
	switch {
	case *promptLoc != "bottom" && *promptLoc != "top":
		return opts, false, fmt.Errorf("--prompt-loc must be bottom or top")
	case f.Changed("prompt-loc") && opts.Prompt == "":
		return opts, false, fmt.Errorf("--prompt-loc needs --prompt")
	case (opts.Output != "" || opts.OutputDir != "") && opts.Prompt != "":
		return opts, false, fmt.Errorf("--prompt cannot be combined with an export, which shows nothing")
	}
	opts.PromptTop = *promptLoc == "top"
	switch {
	case *view != "" && *camera != "":
		return opts, false, fmt.Errorf("choose --view or --camera")
	case f.Changed("view"):
		if opts.Views, err = document.ParseViews(*view); err == nil && len(opts.Views) == 0 {
			err = fmt.Errorf("--view names no view")
		}
	case f.Changed("camera"):
		var at document.View
		at, err = document.ParseCamera(*camera)
		opts.Views = []document.View{at}
	case f.Changed("projection"):
		// The camera there would have been, otherwise projected.
		at := charts.DefaultCamera()
		opts.Views = []document.View{{Alpha: at.Alpha, Beta: at.Beta, Distance: at.Distance}}
	}
	if err != nil {
		return opts, false, fmt.Errorf("--view/--camera: %w", err)
	}
	if !slices.Contains([]string{"ortho", "perspective"}, *projection) {
		return opts, false, fmt.Errorf("--projection must be ortho or perspective")
	}
	for i := range opts.Views {
		if *projection == "perspective" {
			opts.Views[i].Projection = charts.Perspective
		}
	}
	switch {
	case opts.JSON && !opts.Info && !opts.Text && opts.Output == "" && opts.OutputDir == "" && opts.Resume == "":
		return opts, false, fmt.Errorf("--json goes with --info, --text, or an export")
	case opts.Info && (opts.Output != "" || opts.OutputDir != "" || opts.Text || opts.Serve || opts.Pick || opts.Menu || opts.Preview || opts.KeepScreen):
		return opts, false, fmt.Errorf("--info prints and exits; it cannot be combined with the viewer's or export's options")
	case opts.Text && (opts.Serve || opts.Pick || opts.Menu || opts.Preview || opts.KeepScreen || opts.FetchAllowed):
		return opts, false, fmt.Errorf("--text prints and exits; it cannot be combined with the viewer's options")
	}
	if opts.VisionProfile != "" {
		if opts.MaxEdge, opts.EdgeReason, err = document.ResolveEdge(opts.MaxEdge, f.Changed("max-edge"), opts.VisionProfile); err != nil {
			return opts, false, err
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
	if (opts.Output != "" || opts.OutputDir != "") && opts.KeepScreen {
		return opts, false, fmt.Errorf("export draws nothing; it cannot be combined with --no-alt-screen")
	}
	if (opts.Output != "" || opts.OutputDir != "") && (opts.Serve || opts.Pick || opts.FetchAllowed) {
		return opts, false, fmt.Errorf("export cannot be combined with --serve, --pick, or --fetch")
	}
	if opts.Serve && opts.KeepScreen {
		return opts, false, fmt.Errorf("--serve draws on a page; it cannot be combined with --no-alt-screen")
	}
	if opts.NoOpen && !opts.Serve {
		return opts, false, fmt.Errorf("--no-open requires --serve")
	}
	if opts.Timeout < 0 || (opts.Timeout > 0 && !opts.Serve && !opts.Pick && opts.Resume == "") {
		return opts, false, fmt.Errorf("--timeout must be positive, and requires --serve, --pick, or --resume")
	}
	switch {
	case opts.Resume != "" && (len(opts.Files) > 0 || opts.Serve || opts.Pick || opts.Info || opts.Text || opts.Output != "" || opts.OutputDir != "" || opts.Detached != "" || len(opts.Globs) > 0):
		return opts, false, fmt.Errorf("--resume only asks what a pick came to: it takes no files and no other mode (--json, --timeout excepted)")
	case opts.Status != "" && (len(opts.Files) > 0 || opts.Serve || opts.Pick || opts.Info || opts.Text || opts.JSON || opts.Output != "" || opts.OutputDir != "" || opts.Detached != "" || opts.Resume != "" || opts.Timeout > 0 || opts.Prompt != "" || len(opts.Globs) > 0):
		return opts, false, fmt.Errorf("--status only asks how a pick stands: it takes no files, no timeout, and no other mode")
	case opts.Detached != "" && (!opts.Serve || !tokenPattern.MatchString(opts.Detached)):
		return opts, false, fmt.Errorf("--detached is for gloss's own use")
	}
	return opts, false, nil
}

func run(args []string) (err error) {
	opts, done, err := parse(args, os.Stdout)
	if done {
		return nil
	}
	if err != nil {
		return err
	}
	if opts.SkillInstall != "" {
		return installSkill(opts.SkillInstall, os.Stdout, os.Stderr)
	}
	if opts.Status != "" {
		return status(opts, os.Stdout)
	}
	if opts.Resume != "" {
		return resume(opts, os.Stdout, os.Stderr)
	}
	if opts.Detached != "" {
		// The server of a detached start: it answers through its state.
		defer func() { conclude(opts.Detached, err, picked, opts.Pick) }()
	}
	stdinTTY := term.IsTerminal(os.Stdin.Fd())
	opts.MarkdownBase, err = os.Getwd()
	if err != nil {
		return err
	}
	opts.Files = arguments(opts, stdinTTY)
	exporting := opts.Output != "" || opts.OutputDir != "" || opts.Text
	if opts.Output == "-" && term.IsTerminal(os.Stdout.Fd()) {
		return fmt.Errorf("redirect PNG stdout to a file or pipe")
	}
	// With no terminal to answer, a pick or a page must not hold its
	// caller: the server runs apart and the start says where it is.
	if (opts.Pick || opts.Serve) && !stdinTTY && opts.Detached == "" && !exporting && !opts.Info {
		return detach(args, opts, os.Stdout, os.Stderr)
	}
	// A page needs no terminal. A pick keeps standard output for its answer,
	// and draws on the terminal itself.
	var screen *os.File
	switch {
	case exporting || opts.Info || opts.Serve || term.IsTerminal(os.Stdout.Fd()):
	case opts.Pick:
		if screen, err = os.OpenFile("/dev/tty", os.O_WRONLY, 0); err != nil {
			return fmt.Errorf("--pick found no terminal to draw on; use --serve to show a page instead")
		}
		defer screen.Close()
	default:
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
				opts.Stdin = stdinPath
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
	if len(opts.Globs) > 0 {
		if opts.Files, err = expandGlobs(opts.Files, opts.Globs, os.Stderr); err != nil {
			return err
		}
	}
	if opts.Info {
		return describe(opts, os.Stdout, os.Stderr)
	}
	opts.Browse, opts.Files, err = inputs(opts.Files, opts.Type, exporting, os.Stderr)
	if err != nil {
		return err
	}

	if opts.Text {
		return textFiles(opts, os.Stdout, os.Stderr)
	}
	if exporting {
		return exportFiles(opts, os.Stdout, os.Stderr)
	}
	if opts.FetchAllowed {
		// Fetched files are the session's; a picked one is the caller's.
		dir, err := os.MkdirTemp("", "gloss-fetch-")
		if err != nil {
			return err
		}
		opts.Fetch = func(address string) (string, error) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			return document.Fetch(ctx, address, dir)
		}
		defer func() { discardFetched(dir, picked) }()
	}
	if opts.Serve {
		return served(opts, os.Stdout, os.Stderr)
	}
	m := app.New(opts.Options)
	defer m.Close()
	var drawn []tea.ProgramOption
	if screen != nil {
		drawn = append(drawn, tea.WithOutput(screen))
	}
	ended := make(chan error, 1)
	// Bubble Tea opens the controlling TTY automatically when stdin is a pipe.
	program := tea.NewProgram(m, drawn...)
	go func() {
		_, err := program.Run()
		ended <- err
	}()
	var late <-chan time.Time
	if opts.Timeout > 0 {
		late = time.After(opts.Timeout)
	}
	select {
	case err = <-ended:
	case <-late:
		program.Kill()
		<-ended
		return errTimeout
	}
	if err != nil {
		return err
	}
	return finish(m, opts, os.Stdout, os.Stderr)
}

// What the user picked, which a fetched file may be among.
var picked []string

// discardFetched empties the folder of fetched files, except for what was
// picked, and removes the folder once nothing is left in it.
func discardFetched(dir string, picked []string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if path := filepath.Join(dir, entry.Name()); !slices.Contains(picked, path) {
			os.Remove(path)
		}
	}
	if left, err := os.ReadDir(dir); err == nil && len(left) == 0 {
		os.Remove(dir)
	}
}

// arguments are the files named, or standard input when a document is piped
// in and none is. A program that starts gloss to ask for a file pipes in
// nothing: there, standard input is read only if it is named.
func arguments(opts options, terminal bool) []string {
	if len(opts.Files) == 0 && !terminal && !opts.Serve && !opts.Pick {
		return []string{"-"}
	}
	return opts.Files
}

// served shows the viewer on a page, and waits for it to be done with.
func served(opts options, stdout, stderr io.Writer) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	s, err := serve(ctx, opts.Options)
	if err != nil {
		return err
	}
	defer s.Close()
	if opts.Detached != "" {
		// Drops land in the folder the start made, and its state says where we are.
		dir, _ := detachedPaths(opts.Detached)
		s.useFolder(dir)
		announce(opts.Detached, s.URL)
	}
	fmt.Fprintf(stderr, "gloss: viewer at %s\n", s.URL)
	if !opts.NoOpen {
		if err := browse(s.URL); err != nil {
			fmt.Fprintf(stderr, "gloss: no browser could be opened (%v); open the address yourself\n", err)
		}
	}
	go func() {
		<-ctx.Done()
		s.Close()
		s.ended.Do(func() { close(s.done) })
	}()
	m, err := s.Wait(opts.Timeout)
	if err == nil && m == nil {
		err = errCancelled
	}
	if err == nil {
		defer m.Close()
		err = finish(m, opts, stdout, stderr)
	}
	if err != nil || !opts.Pick {
		s.Discard()
	}
	return err
}

// finish reports what came of the viewer once the screen is given back:
// what was skipped, and the paths a pick was waiting for.
func finish(m *app.Model, opts options, stdout, stderr io.Writer) error {
	picked = m.Picked()
	for _, line := range m.Skipped() {
		fmt.Fprintln(stderr, line)
	}
	if err := m.Err(); err != nil && !opts.Pick {
		return err
	}
	if !opts.Pick {
		return nil
	}
	if len(m.Picked()) == 0 {
		return errCancelled
	}
	for _, path := range m.Picked() {
		fmt.Fprintln(stdout, path)
	}
	return nil
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

// expandGlobs searches each folder named, or the working folder when none
// is, for the files the patterns match, and puts them in the folder's
// place; files named stay as they are. The search keeps to the limits
// gloss sets for one, and says on stderr where it stopped short.
func expandGlobs(paths, patterns []string, stderr io.Writer) ([]string, error) {
	var out []string
	folders := 0
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			folders++
			found, err := document.Find(path, patterns, document.Limits{})
			if err != nil {
				return nil, err
			}
			if found.Note != "" {
				fmt.Fprintf(stderr, "gloss: %s: %s\n", path, found.Note)
			}
			if len(found.Paths) == 0 {
				fmt.Fprintf(stderr, "gloss: %s: no match for %s\n", path, strings.Join(patterns, " "))
			}
			out = append(out, found.Paths...)
			continue
		}
		out = append(out, path)
	}
	if folders == 0 && len(paths) == 0 {
		return expandGlobs([]string{"."}, patterns, stderr)
	}
	return out, nil
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
