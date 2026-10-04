package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/NimbleMarkets/gloss/internal/app"
)

// conforms checks a decoded value against the part of JSON Schema that
// writeSchema uses: $ref, type, required, properties, items, enum, const. A
// property the schema does not name is an error too, so that a field added
// to the output and not to its type cannot pass.
func conforms(root, s map[string]any, v any, at string) error {
	if ref, ok := s["$ref"].(string); ok {
		return conforms(root, root["$defs"].(map[string]any)[strings.TrimPrefix(ref, "#/$defs/")].(map[string]any), v, at)
	}
	types := []string{}
	switch t := s["type"].(type) {
	case string:
		types = append(types, t)
	case []any:
		for _, x := range t {
			types = append(types, x.(string))
		}
	}
	kind := ""
	switch x := v.(type) {
	case nil:
		kind = "null"
	case bool:
		kind = "boolean"
	case float64:
		kind = "number"
		if x == float64(int64(x)) {
			kind = "integer"
		}
	case string:
		kind = "string"
	case []any:
		kind = "array"
	case map[string]any:
		kind = "object"
	}
	if len(types) > 0 && !slices.Contains(types, kind) && !(kind == "integer" && slices.Contains(types, "number")) {
		return fmt.Errorf("%s: %s, want %v", at, kind, types)
	}
	if c, ok := s["const"]; ok && c != v {
		return fmt.Errorf("%s: %v, want %v", at, v, c)
	}
	if enum, ok := s["enum"].([]any); ok && !slices.Contains(enum, v) {
		return fmt.Errorf("%s: %v not in %v", at, v, enum)
	}
	switch x := v.(type) {
	case []any:
		for i, item := range x {
			if err := conforms(root, s["items"].(map[string]any), item, fmt.Sprintf("%s[%d]", at, i)); err != nil {
				return err
			}
		}
	case map[string]any:
		required, _ := s["required"].([]any)
		for _, name := range required {
			if _, ok := x[name.(string)]; !ok {
				return fmt.Errorf("%s: no %s", at, name)
			}
		}
		props, _ := s["properties"].(map[string]any)
		extra, _ := s["additionalProperties"].(map[string]any)
		for name, value := range x {
			p, ok := props[name].(map[string]any)
			if !ok {
				p = extra
			}
			if p == nil {
				return fmt.Errorf("%s: %s is not in the schema", at, name)
			}
			if err := conforms(root, p, value, at+"."+name); err != nil {
				return err
			}
		}
	}
	return nil
}

func TestSchemaDescribesWhatGlossWrites(t *testing.T) {
	var out bytes.Buffer
	if _, done, err := parse([]string{"skill", "schema"}, &out); err != nil || !done {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(out.Bytes(), &root); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	check := func(def string, data []byte) {
		t.Helper()
		var v any
		if err := json.Unmarshal(data, &v); err != nil {
			t.Fatalf("%s: %v %s", def, err, data)
		}
		if err := conforms(root, map[string]any{"$ref": "#/$defs/" + def}, v, def); err != nil {
			t.Errorf("%v\n%s", err, data)
		}
	}
	run := func(opts options, f func(options, *bytes.Buffer, *bytes.Buffer) error) []byte {
		var stdout, stderr bytes.Buffer
		_ = f(opts, &stdout, &stderr)
		return stdout.Bytes()
	}
	files := []string{"../../examples/field-guide.pdf", "../../examples/shapes.svg", "../../examples/sales.xlsx", "../../examples/nope.pdf"}
	text := func(o options, a, b *bytes.Buffer) error { return textFiles(o, a, b) }
	check("results", run(options{Options: app.Options{Files: files, Page: 1, DPI: 72, MaxEdge: 64}, Text: true, JSON: true}, text))
	figure := figurePDF(t, "BT /F1 12 Tf 10 10 Td (Fig. 1) Tj ET q /Im1 Do Q", "q /Im1 Do Q")
	check("results", run(options{Options: app.Options{Files: []string{figure}, DPI: 72}, Text: true, JSON: true, Pages: pageRange{all: true}}, text))
	dir := t.TempDir()
	check("results", run(options{Options: app.Options{Files: files, Page: 1, DPI: 72, MaxEdge: 64, OutputDir: dir}, JSON: true, VisionProfile: "claude-standard", EdgeReason: "x"},
		func(o options, a, b *bytes.Buffer) error { return exportFiles(o, a, b) }))
	check("results", run(options{Options: app.Options{Files: append(files, figure), DPI: 72}, JSON: true, Grep: grepPattern("(?i)page"), Pages: pageRange{all: true}},
		func(o options, a, b *bytes.Buffer) error { return grepFiles(o, a, b) }))
	check("info", run(options{Options: app.Options{Files: files, Page: 1, DPI: 72}, Info: true, JSON: true},
		func(o options, a, b *bytes.Buffer) error { return describe(o, a, b) }))

	left := 3
	for _, s := range []sessionStatus{{Protocol: 1, State: "waiting", SecondsLeft: &left}, {Protocol: 1, State: "picked", Settled: true, Paths: []string{"/a"}}, {Protocol: 1, State: "failed", Settled: true, Error: "x"}} {
		data, _ := json.Marshal(s)
		check("status", data)
	}
	data, _ := json.Marshal(started{Protocol: 1, Status: statusWaiting, URL: "http://127.0.0.1:1/x/", Dir: "/d", TimeoutSeconds: 600, ResumeToken: testToken, Resume: "gloss --resume " + testToken, Pick: true})
	check("started", data)
	for _, st := range []state{{Status: statusPicked, Paths: []string{"/a"}}, {Status: statusDeclined}} {
		var b bytes.Buffer
		_ = answer(st, true, &b)
		check("resumed", b.Bytes())
	}
	// What a session really prints, through --status.
	settle(t, testToken, state{Status: statusDeclined})
	var b bytes.Buffer
	if err := status(options{Status: testToken}, &b); err != nil {
		t.Fatal(err)
	}
	check("status", b.Bytes())
	// And the version stated is the schema's.
	if !strings.Contains(root["title"].(string), version) {
		t.Errorf("title %q", root["title"])
	}
}

func TestSkillMentionsTheSchema(t *testing.T) {
	var out bytes.Buffer
	if _, _, err := parse([]string{"skill"}, &out); err != nil || !strings.Contains(out.String(), "gloss skill schema") {
		t.Fatalf("the skill does not mention gloss skill schema: %v", err)
	}
}
