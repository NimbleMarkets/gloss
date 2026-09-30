package document

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Notebook is a Jupyter notebook laid out as Markdown: its Markdown cells
// as they are, its code cells fenced, and their outputs after them. The
// pictures among the outputs are files in Files.
type Notebook struct {
	Markdown []byte
	Files    *Overlay
	format   string
	language string
	kernel   string
	cells    map[string]int
	outputs  int
	pictures int
	cut      bool
}

// The parts of the nbformat 4 schema that are shown.
type nbRaw struct {
	Cells    []nbCell `json:"cells"`
	Metadata struct {
		Kernelspec struct {
			DisplayName string `json:"display_name"`
			Language    string `json:"language"`
		} `json:"kernelspec"`
		LanguageInfo struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"language_info"`
	} `json:"metadata"`
	Format      int `json:"nbformat"`
	FormatMinor int `json:"nbformat_minor"`
}

type nbCell struct {
	Type    string     `json:"cell_type"`
	Source  nbText     `json:"source"`
	Count   *int       `json:"execution_count"`
	Outputs []nbOutput `json:"outputs"`
}

type nbOutput struct {
	Type      string            `json:"output_type"`
	Text      nbText            `json:"text"`
	Data      map[string]nbText `json:"data"`
	Name      string            `json:"ename"`
	Value     string            `json:"evalue"`
	Traceback []string          `json:"traceback"`
}

// nbText is a string, or a list of lines, as the schema allows either.
type nbText string

func (t *nbText) UnmarshalJSON(data []byte) error {
	var s string
	if json.Unmarshal(data, &s) == nil {
		*t = nbText(s)
		return nil
	}
	var lines []string
	if err := json.Unmarshal(data, &lines); err != nil {
		return err
	}
	*t = nbText(strings.Join(lines, ""))
	return nil
}

// The output data shown, the first found taken: a picture over text.
var nbShown = []string{"text/markdown", "image/png", "image/jpeg", "image/gif", "image/svg+xml", "text/plain"}

// ReadNotebook lays out a notebook in nbformat 4.
func ReadNotebook(data []byte) (*Notebook, error) {
	var raw nbRaw
	if err := json.Unmarshal(bytes.TrimPrefix(data, []byte("\ufeff")), &raw); err != nil {
		return nil, fmt.Errorf("not a notebook: %w", err)
	}
	if raw.Format != 4 {
		return nil, fmt.Errorf("only nbformat 4 notebooks are read, not %d", raw.Format)
	}
	nb := &Notebook{Files: &Overlay{}, cells: map[string]int{}}
	nb.format = fmt.Sprintf("Jupyter notebook %d.%d", raw.Format, raw.FormatMinor)
	nb.language = strings.TrimSpace(raw.Metadata.LanguageInfo.Name + " " + raw.Metadata.LanguageInfo.Version)
	if nb.language == "" {
		nb.language = raw.Metadata.Kernelspec.Language
	}
	nb.kernel = raw.Metadata.Kernelspec.DisplayName
	fence := raw.Metadata.LanguageInfo.Name
	if fence == "" {
		fence = raw.Metadata.Kernelspec.Language
	}
	var md bytes.Buffer
	for i, cell := range raw.Cells {
		nb.cells[cell.Type]++
		source := strings.TrimRight(string(cell.Source), "\n")
		switch cell.Type {
		case "markdown":
			md.WriteString(source + "\n\n")
		case "code":
			if source == "" && len(cell.Outputs) == 0 {
				continue
			}
			if cell.Count != nil {
				fmt.Fprintf(&md, "**In [%d]:**\n\n", *cell.Count)
			}
			fmt.Fprintf(&md, "```%s\n%s\n```\n\n", fence, source)
			for j, out := range cell.Outputs {
				nb.outputs++
				nb.output(&md, out, i+1, j+1)
			}
		default:
			if source != "" {
				fmt.Fprintf(&md, "```\n%s\n```\n\n", source)
			}
		}
		if md.Len() > MaxMarkdownBytes-4096 {
			nb.cut = true
			fmt.Fprintf(&md, "*First %d of %d cells shown.*\n", i+1, len(raw.Cells))
			break
		}
	}
	nb.Markdown = md.Bytes()
	return nb, nil
}

// output writes one output of a code cell: a picture as a file to embed,
// text fenced so that it stays as it is.
func (nb *Notebook) output(md *bytes.Buffer, out nbOutput, cell, n int) {
	switch out.Type {
	case "stream":
		fenced(md, "text", string(out.Text))
	case "error":
		lines := make([]string, len(out.Traceback))
		for i, line := range out.Traceback {
			lines[i] = ansi.Strip(line)
		}
		text := strings.Join(lines, "\n")
		if text == "" {
			text = strings.TrimSpace(out.Name + ": " + out.Value)
		}
		fenced(md, "text", text)
	case "execute_result", "display_data":
		for _, mime := range nbShown {
			data, ok := out.Data[mime]
			if !ok {
				continue
			}
			switch mime {
			case "text/markdown":
				md.WriteString(strings.TrimRight(string(data), "\n") + "\n\n")
			case "text/plain":
				fenced(md, "text", string(data))
			default:
				ext := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/svg+xml": ".svg"}[mime]
				content := []byte(data)
				if mime != "image/svg+xml" {
					decoded, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(string(data)), ""))
					if err != nil {
						continue
					}
					content = decoded
				}
				name, err := nb.Files.Add(fmt.Sprintf("cell-%d-output-%d%s", cell, n, ext), content)
				if err != nil {
					continue
				}
				nb.pictures++
				fmt.Fprintf(md, "![output](%s)\n\n", name)
			}
			return
		}
	}
}

// fenced writes text in a code block, its fence longer than any run of
// backticks in the text.
func fenced(md *bytes.Buffer, language, text string) {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return
	}
	fence := "```"
	for strings.Contains(text, fence) {
		fence += "`"
	}
	fmt.Fprintf(md, "%s%s\n%s\n%s\n\n", fence, language, text, fence)
}

func (nb *Notebook) fields() []Field {
	cells := nb.cells["code"] + nb.cells["markdown"] + nb.cells["raw"]
	var parts []string
	for _, kind := range []string{"code", "markdown", "raw"} {
		if n := nb.cells[kind]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, kind))
		}
	}
	count := fmt.Sprintf("%d (%s)", cells, strings.Join(parts, ", "))
	if nb.cut {
		count += ", not all shown"
	}
	return Section("Notebook",
		Field{"Format", nb.format},
		Field{"Language", nb.language},
		Field{"Kernel", nb.kernel},
		Field{"Cells", count},
		Field{"Outputs", grouped(nb.outputs)},
		Field{"Pictures", grouped(nb.pictures)},
	)
}
