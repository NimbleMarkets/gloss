package document

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// The Markdown made from JSON stays under the document limit, with room for
// the note that says it was cut.
const maxJSONMarkdown = MaxMarkdownBytes - 4096

// JSONDoc is a JSON file, or a file of JSON lines, laid out as Markdown:
// each value pretty-printed in a fenced block for highlighting.
type JSONDoc struct {
	Markdown []byte
	Lines    bool // One value to a line, as in JSONL and NDJSON.
	Records  int  // Values in a file of lines.
	Shown    int  // Records or lines that fit the Markdown.
	kind     string
	count    int
	keys     []string
	cut      bool
}

// ReadJSON pretty-prints data. A file of several values, one to a line, is
// read as records whatever its name, and an error says where the JSON
// stopped making sense.
func ReadJSON(path string, data []byte) (*JSONDoc, error) {
	data = bytes.TrimPrefix(data, []byte("\ufeff"))
	doc := &JSONDoc{}
	if json.Valid(data) {
		return doc.single(data)
	}
	lines := bytes.Split(data, []byte("\n"))
	if ext := strings.ToLower(filepath.Ext(path)); ext == ".json" && !looksLikeLines(lines) {
		return nil, jsonError(data)
	}
	return doc.records(lines)
}

// looksLikeLines says whether the first two values of the file each keep to
// a line: then a .json file is read as lines too.
func looksLikeLines(lines [][]byte) bool {
	n := 0
	for _, line := range lines {
		if line = bytes.TrimSpace(line); len(line) > 0 {
			if !json.Valid(line) {
				return false
			}
			n++
		}
	}
	return n > 1
}

func (doc *JSONDoc) single(data []byte) (*JSONDoc, error) {
	doc.kind, doc.count, doc.keys = topLevel(data)
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, bytes.TrimSpace(data), "", "  "); err != nil {
		return nil, err
	}
	lines := bytes.Count(pretty.Bytes(), []byte("\n")) + 1
	shown := lines
	if pretty.Len() > maxJSONMarkdown {
		cut := bytes.LastIndexByte(pretty.Bytes()[:maxJSONMarkdown], '\n')
		pretty.Truncate(max(0, cut))
		shown = bytes.Count(pretty.Bytes(), []byte("\n")) + 1
		doc.cut = true
	}
	var md bytes.Buffer
	md.WriteString("```json\n")
	md.Write(pretty.Bytes())
	md.WriteString("\n```\n")
	if doc.cut {
		fmt.Fprintf(&md, "\n*First %s of %s lines shown.*\n", grouped(shown), grouped(lines))
	}
	doc.Markdown, doc.Shown = md.Bytes(), shown
	return doc, nil
}

func (doc *JSONDoc) records(lines [][]byte) (*JSONDoc, error) {
	doc.Lines = true
	var md bytes.Buffer
	for i, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if !json.Valid(line) {
			return nil, fmt.Errorf("line %d is not JSON", i+1)
		}
		doc.Records++
		if doc.Records == 1 {
			doc.kind, doc.count, doc.keys = topLevel(line)
		}
		if doc.cut {
			continue
		}
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, line, "", "  "); err != nil {
			return nil, fmt.Errorf("line %d: %w", i+1, err)
		}
		if md.Len()+pretty.Len() > maxJSONMarkdown {
			doc.cut = true
			continue
		}
		fmt.Fprintf(&md, "#### %d\n\n```json\n%s\n```\n\n", doc.Records, pretty.Bytes())
		doc.Shown++
	}
	if doc.cut {
		fmt.Fprintf(&md, "*First %s of %s records shown.*\n", grouped(doc.Shown), grouped(doc.Records))
	}
	doc.Markdown = md.Bytes()
	return doc, nil
}

// jsonError says on which line the JSON went wrong.
func jsonError(data []byte) error {
	err := json.Unmarshal(data, new(any))
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) {
		line := bytes.Count(data[:min(int(syntax.Offset), len(data))], []byte("\n")) + 1
		return fmt.Errorf("not JSON: line %d: %s", line, syntax.Error())
	}
	if err == nil {
		err = fmt.Errorf("not JSON")
	}
	return err
}

// topLevel describes the outermost value: its kind, how many members it
// has, and the first keys of an object.
func topLevel(data []byte) (kind string, count int, keys []string) {
	dec := json.NewDecoder(bytes.NewReader(data))
	first, err := dec.Token()
	if err != nil {
		return "", 0, nil
	}
	switch first {
	case json.Delim('{'):
		kind = "object"
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				break
			}
			var value json.RawMessage
			if dec.Decode(&value) != nil {
				break
			}
			count++
			if name, ok := key.(string); ok && len(keys) < 8 {
				keys = append(keys, name)
			}
		}
	case json.Delim('['):
		kind = "array"
		for dec.More() {
			var value json.RawMessage
			if dec.Decode(&value) != nil {
				break
			}
			count++
		}
	default:
		kind = fmt.Sprintf("%T", first)
		switch first.(type) {
		case string:
			kind = "string"
		case float64, json.Number:
			kind = "number"
		case bool:
			kind = "boolean"
		case nil:
			kind = "null"
		}
	}
	return kind, count, keys
}

func (doc *JSONDoc) fields() []Field {
	top := doc.kind
	switch doc.kind {
	case "object":
		top = "object with " + plural(doc.count, "key")
	case "array":
		top = fmt.Sprintf("array of %s", grouped(doc.count))
	}
	fields := []Field{{"Format", "JSON"}}
	if doc.Lines {
		fields = []Field{{"Format", "JSON Lines"}, {"Records", grouped(doc.Records)}}
		if doc.cut {
			fields = append(fields, Field{"Shown", grouped(doc.Shown)})
		}
		fields = append(fields, Field{"Record", top}, Field{"Record keys", strings.Join(doc.keys, ", ")})
	} else {
		fields = append(fields, Field{"Top level", top}, Field{"Keys", strings.Join(doc.keys, ", ")})
	}
	return Section("JSON", fields...)
}
