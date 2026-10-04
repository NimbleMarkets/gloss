package main

import (
	"encoding/json"
	"io"
	"reflect"
	"strings"
)

// The schema of what gloss writes for a program to read is made from the very
// types it encodes, so the two cannot drift apart: a field's json tag names
// it, omitempty makes it optional, and two more tags say what the type cannot,
// doc a description and enum or const the values it takes.

// schemaShapes are the objects gloss writes, by the name the schema gives each.
var schemaShapes = []struct {
	name, doc string
	value     any
	array     bool
}{
	{"results", "What --json prints with --text, --grep, --output, or --output-dir: one object per input and page, or per match with --grep; failed ones carry error.", made{}, true},
	{"info", "What --info --json prints: one object per file; failed ones carry error.", described{}, true},
	{"started", "The one object --pick or --serve prints, and exits 0, when standard input is not a terminal.", started{}, false},
	{"status", "The one object --status prints.", sessionStatus{}, false},
	{"resumed", "The one object --resume --json prints.", resumed{}, false},
}

// writeSchema prints the JSON Schema of the shapes, as one document: each in
// $defs, and the document as any one of them.
func writeSchema(out io.Writer) error {
	defs := map[string]any{}
	var oneOf []any
	for _, shape := range schemaShapes {
		s := schemaOf(reflect.TypeOf(shape.value))
		if shape.array {
			item := shape.name + "_item"
			defs[item] = s
			s = map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/" + item}}
		}
		s["description"] = shape.doc
		defs[shape.name] = s
		oneOf = append(oneOf, map[string]any{"$ref": "#/$defs/" + shape.name})
	}
	doc := map[string]any{
		"$schema":     "https://json-schema.org/draft/2020-12/schema",
		"title":       "gloss " + version + " output",
		"description": "The JSON gloss writes on standard output for a program. Objects of a detached session carry protocol, the version of their shape; new fields may appear without it changing.",
		"anyOf":       oneOf,
		"$defs":       defs,
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

func schemaOf(t reflect.Type) map[string]any {
	switch t.Kind() {
	case reflect.Pointer:
		return schemaOf(t.Elem()) // Absent, not null: these are all omitempty.
	case reflect.Struct:
		props, required := map[string]any{}, []string{}
		for i := range t.NumField() {
			field := t.Field(i)
			tag := field.Tag.Get("json")
			if !field.IsExported() || tag == "-" {
				continue
			}
			name, opts, _ := strings.Cut(tag, ",")
			if name == "" {
				name = field.Name
			}
			s := schemaOf(field.Type)
			omitted := strings.Contains(opts, "omitempty")
			if k := field.Type.Kind(); !omitted && (k == reflect.Slice || k == reflect.Map) {
				s["type"] = []string{s["type"].(string), "null"}
			}
			if doc := field.Tag.Get("doc"); doc != "" {
				s["description"] = doc
			}
			if enum := field.Tag.Get("enum"); enum != "" {
				s["enum"] = strings.Split(enum, ",")
			}
			if c := field.Tag.Get("const"); c != "" {
				var v any
				if json.Unmarshal([]byte(c), &v) == nil {
					s["const"] = v
				}
			}
			props[name] = s
			if !omitted {
				required = append(required, name)
			}
		}
		return map[string]any{"type": "object", "properties": props, "required": required}
	case reflect.Slice, reflect.Array:
		return map[string]any{"type": "array", "items": schemaOf(t.Elem())}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": schemaOf(t.Elem())}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	}
	return map[string]any{}
}
