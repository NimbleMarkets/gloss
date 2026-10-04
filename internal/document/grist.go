package document

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	sdb "github.com/neomantra/sqlittle/db"
)

// ErrNotGrist is said of a SQLite database that is not a Grist document.
// gloss is not a browser of databases: it reads the tables Grist describes.
// It is an unsupported format, with a reason of its own.
var ErrNotGrist error = notGrist{}

type notGrist struct{}

func (notGrist) Error() string        { return "a SQLite database, but not a Grist document" }
func (notGrist) Is(target error) bool { return target == ErrUnsupported }

const sqliteMagic = "SQLite format 3\x00"

// maxGristCatalog bounds the rows read of each table that describes the
// others.
const maxGristCatalog = 1 << 20

// GristDoc is a Grist document: a SQLite database whose own tables say which
// of the others are the user's, and what their columns are called. Its tables
// are read as sheets, as they are turned to. Only what is stored is read:
// formulas are Grist's to work out, and it keeps their answers in the file.
type GristDoc struct {
	db      *sdb.Database
	tables  []gristTable
	columns map[int64]gristColumn // Of every table, by the catalog's id.
	files   map[int64]string      // The names of attachments.
	zone    string
	version int64
	zones   map[string]*time.Location
	sheets  []*Sheet
	errs    []error
	shown   map[[2]int64]map[int64]any // What rows of another table show, by table and column, for references.
}

type gristTable struct {
	id      int64
	name    string // In SQLite.
	title   string // As Grist shows it; empty for the name.
	summary bool
	source  int64         // The table a summary sums.
	columns []gristColumn // In the order Grist shows them.
}

type gristColumn struct {
	id       int64
	table    int64
	pos      float64
	name     string // In SQLite.
	label    string
	kind     string // Text, Int, Date, Ref:Table, and so on.
	display  int64  // The column holding what a reference shows, or zero.
	visible  int64  // The column of the other table that it shows.
	source   int64  // In a summary, the column it groups by, of the table summed.
	link     bool   // Shown as a hyperlink: words, then the address they lead to.
	position int    // Among the columns SQLite stores, or -1.
}

// isGrist says whether data is a SQLite database with Grist's catalog in it.
// A database cut short may hold the catalog beyond what is given.
func isGrist(data []byte) (ok bool) {
	if !bytes.HasPrefix(data, []byte(sqliteMagic)) {
		return false
	}
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	db, err := sdb.OpenBytes(data)
	if err != nil {
		return false
	}
	defer db.Close()
	_, err = db.Table("_grist_Tables")
	return err == nil
}

// OpenGrist reads the catalog of a Grist document.
func OpenGrist(data []byte) (*GristDoc, error) {
	if !bytes.HasPrefix(data, []byte(sqliteMagic)) {
		return nil, fmt.Errorf("not a Grist document")
	}
	db, err := sdb.OpenBytes(data)
	if err != nil {
		return nil, fmt.Errorf("grist document: %w", err)
	}
	if _, err := db.Table("_grist_Tables"); err != nil {
		return nil, ErrNotGrist
	}
	g := &GristDoc{db: db, columns: map[int64]gristColumn{}, files: map[int64]string{}}

	// The titles, attachments, and settings are courtesies: a document
	// without them is still read.
	titles := map[int64]string{}
	_ = g.catalog("_grist_Views_section", func(id int64, get func(string) any) {
		titles[id] = oneLine(gristText(get("title")))
	})
	_ = g.catalog("_grist_Attachments", func(id int64, get func(string) any) {
		g.files[id] = oneLine(gristText(get("fileName")))
	})
	_ = g.catalog("_grist_DocInfo", func(id int64, get func(string) any) {
		g.zone, g.version = gristText(get("timezone")), gristInt(get("schemaVersion"))
	})

	byTable := map[int64][]gristColumn{}
	if err := g.catalog("_grist_Tables_column", func(id int64, get func(string) any) {
		c := gristColumn{id: id, table: gristInt(get("parentId")), pos: gristFloat(get("parentPos")), name: gristText(get("colId")),
			label: oneLine(gristText(get("label"))), kind: gristText(get("type")), display: gristInt(get("displayCol")), visible: gristInt(get("visibleCol")), source: gristInt(get("summarySourceCol")), position: -1}
		var options struct{ Widget string }
		if json.Unmarshal([]byte(gristText(get("widgetOptions"))), &options) == nil {
			c.link = options.Widget == "HyperLink"
		}
		g.columns[id] = c
		byTable[c.table] = append(byTable[c.table], c)
	}); err != nil {
		return nil, fmt.Errorf("grist document: columns: %w", err)
	}
	var summaries []gristTable
	if err := g.catalog("_grist_Tables", func(id int64, get func(string) any) {
		t := gristTable{id: id, name: gristText(get("tableId")), title: titles[gristInt(get("rawViewSectionRef"))], summary: gristInt(get("summarySourceTable")) != 0, source: gristInt(get("summarySourceTable")), columns: byTable[id]}
		if t.name == "" || strings.HasPrefix(t.name, "_grist") {
			return
		}
		sort.SliceStable(t.columns, func(i, j int) bool { return t.columns[i].pos < t.columns[j].pos })
		// A summary is Grist's own reckoning of another table: after the
		// tables people made.
		if t.summary {
			summaries = append(summaries, t)
		} else {
			g.tables = append(g.tables, t)
		}
	}); err != nil {
		return nil, fmt.Errorf("grist document: tables: %w", err)
	}
	for _, t := range summaries {
		g.tables = append(g.tables, g.summarize(t))
	}
	if len(g.tables) == 0 {
		return nil, fmt.Errorf("grist document has no tables")
	}
	g.sheets, g.errs = make([]*Sheet, len(g.tables)), make([]error, len(g.tables))
	return g, nil
}

// summarize finds, for a summary, the columns it groups by among those of
// the table it sums, and titles it as Grist does, by that table and them:
// Sightings [by Site].
func (g *GristDoc) summarize(t gristTable) gristTable {
	var source gristTable
	for _, other := range g.tables {
		if other.id == t.source {
			source = other
		}
	}
	var by []string
	for i, c := range t.columns {
		// Grist names the column as the one it groups by; an older document
		// may not say which that is.
		for _, s := range source.columns {
			if c.source == 0 && s.name == c.name {
				c.source = s.id
			}
		}
		if c.source != 0 {
			t.columns[i] = c
			by = append(by, cmp.Or(c.label, c.name))
		}
	}
	if t.title == "" && source.id != 0 && len(by) > 0 {
		t.title = cmp.Or(source.title, source.name) + " [by " + strings.Join(by, ", ") + "]"
	}
	return t
}

// catalog reads a table of the catalog, each row by the names of its columns.
func (g *GristDoc) catalog(name string, each func(id int64, get func(string) any)) error {
	table, at, err := g.table(name)
	if err != nil {
		return err
	}
	rows := 0
	err = table.Scan(func(id int64, rec sdb.Record) bool {
		if rows++; rows > maxGristCatalog {
			return true
		}
		each(id, func(column string) any {
			if i, ok := at[column]; ok && i < len(rec) {
				return rec[i]
			}
			return nil
		})
		return false
	})
	if err == nil && rows > maxGristCatalog {
		err = fmt.Errorf("more than %s rows", grouped(maxGristCatalog))
	}
	return err
}

// table opens a table and finds where in a row each of its columns is.
func (g *GristDoc) table(name string) (*sdb.Table, map[string]int, error) {
	table, err := g.db.Table(name)
	if err != nil {
		return nil, nil, err
	}
	def, err := table.Def()
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read the definition of %s", name)
	}
	at := make(map[string]int, len(def.Columns))
	for i, c := range def.Columns {
		at[c.Name] = i
	}
	return table, at, nil
}

// Names are the tables as Grist titles them.
func (g *GristDoc) Names() []string {
	names := make([]string, len(g.tables))
	for i, t := range g.tables {
		if names[i] = t.title; names[i] == "" {
			names[i] = t.name
		}
	}
	return names
}

// Sheet reads table i, counting from zero, once.
func (g *GristDoc) Sheet(i int) (*Sheet, error) {
	if i < 0 || i >= len(g.tables) {
		return nil, fmt.Errorf("document has %d tables", len(g.tables))
	}
	if g.sheets[i] == nil && g.errs[i] == nil {
		sheet, err := g.readTable(g.tables[i], g.Names()[i])
		if err != nil {
			err = fmt.Errorf("table %s: %w", g.Names()[i], err)
		}
		g.sheets[i], g.errs[i] = sheet, err
	}
	return g.sheets[i], g.errs[i]
}

// readTable reads a table as a sheet: its labels in the first row, then its
// rows in the order Grist keeps them in. The row ids, the positions rows are
// sorted by, and the columns Grist adds for its own use are left out, as is
// a summary's list of the rows behind each of its own.
func (g *GristDoc) readTable(t gristTable, name string) (*Sheet, error) {
	table, at, err := g.table(t.name)
	if err != nil {
		return nil, err
	}
	place := func(column string) int {
		if i, ok := at[column]; ok {
			return i
		}
		return -1
	}
	sheet := &Sheet{Name: name}
	var shown []gristColumn
	header := []string{}
	for _, c := range t.columns {
		if c.name == "id" || c.name == "manualSort" || strings.HasPrefix(c.name, "gristHelper_") {
			continue
		}
		// A summary keeps the rows it sums in a column Grist does not show.
		if t.summary && c.name == "group" {
			continue
		}
		if len(shown) >= maxSheetColumns {
			sheet.MoreColumns++
			continue
		}
		c.position = place(c.name)
		shown = append(shown, c)
		if c.label == "" {
			c.label = c.name
		}
		header = append(header, c.label)
	}
	// What a reference shows is kept beside it, in a column of the same row.
	displays := make([]gristColumn, len(shown))
	// A summary's has none: it is looked up in the other table, by the column
	// that the summed table's reference shows.
	lookups := make([]map[int64]any, len(shown))
	for i, c := range shown {
		displays[i] = gristColumn{position: -1}
		if d, ok := g.columns[c.display]; ok && d.table == t.id {
			displays[i] = gristColumn{position: place(d.name), kind: g.columns[c.visible].kind}
		} else if visible := cmp.Or(c.visible, g.columns[c.source].visible); strings.HasPrefix(c.kind, "Ref:") && visible != 0 {
			displays[i].kind = g.columns[visible].kind
			lookups[i] = g.showing(strings.TrimPrefix(c.kind, "Ref:"), g.columns[visible])
		}
	}
	type sorted struct {
		at    float64
		id    int64
		cell  []string
		links map[int]string // Addresses, by column.
	}
	var rows []sorted
	order := place("manualSort")
	value := func(rec sdb.Record, i int) any {
		if i >= 0 && i < len(rec) {
			return rec[i]
		}
		return nil
	}
	err = table.Scan(func(id int64, rec sdb.Record) bool {
		if len(rows) >= maxSheetRows-1 {
			sheet.MoreRows++
			return false
		}
		row := sorted{at: math.Inf(1), id: id, cell: make([]string, len(shown))}
		if v := value(rec, order); v != nil {
			row.at = gristFloat(v)
		}
		for i, c := range shown {
			v := value(rec, c.position)
			if d := displays[i]; d.position >= 0 && !gristEmptyRef(v) {
				// A value Grist could not take for a reference stands as
				// written; a list of references is kept as text.
				written, text := v.(string)
				if !text || (strings.HasPrefix(c.kind, "RefList:") && strings.HasPrefix(written, "[")) {
					row.cell[i] = g.cell(d.kind, value(rec, d.position))
				}
			}
			if shows, ok := lookups[i][int64(gristFloat(v))]; ok && !gristEmptyRef(v) {
				row.cell[i] = g.cell(displays[i].kind, shows)
			}
			if row.cell[i] == "" {
				row.cell[i] = g.cell(c.kind, v)
			}
			if c.link {
				if words, address, ok := gristLink(row.cell[i]); ok {
					if row.links == nil {
						row.links = map[int]string{}
					}
					row.cell[i], row.links[i] = words, address
				}
			}
		}
		rows = append(rows, row)
		return false
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].at < rows[j].at })
	sheet.Rows, sheet.Columns = make([][]string, 0, len(rows)+1), len(shown)
	sheet.Rows = append(sheet.Rows, header)
	for _, r := range rows {
		for column, address := range r.links {
			if sheet.links == nil {
				sheet.links = map[[2]int]string{}
			}
			sheet.links[[2]int{len(sheet.Rows), column}] = address
		}
		sheet.Rows = append(sheet.Rows, r.cell)
	}
	return sheet, nil
}

// showing reads what column shows in each row of table, by row id, once.
// A table that cannot be read shows nothing, and its references stand as
// they are.
func (g *GristDoc) showing(table string, column gristColumn) map[int64]any {
	key := [2]int64{column.table, column.id}
	if rows, ok := g.shown[key]; ok {
		return rows
	}
	if g.shown == nil {
		g.shown = map[[2]int64]map[int64]any{}
	}
	rows := map[int64]any{}
	g.shown[key] = rows
	t, at, err := g.table(table)
	i, ok := at[column.name]
	if err != nil || !ok {
		return rows
	}
	_ = t.Scan(func(id int64, rec sdb.Record) bool {
		if len(rows) >= maxSheetRows {
			return true
		}
		if i < len(rec) {
			rows[id] = rec[i]
		}
		return false
	})
	return rows
}

// gristLink parts the text of a hyperlink cell, which is words and then the
// address they lead to, as Grist keeps it. Only an address on the web is
// taken for one, as in a workbook; text that is all address is left to stand.
func gristLink(text string) (words, address string, ok bool) {
	at := strings.LastIndexByte(text, ' ')
	if at < 0 {
		return "", "", false
	}
	words, address = text[:at], text[at+1:]
	if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") {
		return "", "", false
	}
	if u, err := url.Parse(address); err != nil || u.Host == "" {
		return "", "", false
	}
	return words, address, true
}

// gristEmptyRef says whether a reference points at nothing.
func gristEmptyRef(v any) bool {
	switch v := v.(type) {
	case nil:
		return true
	case int64:
		return v == 0
	case float64:
		return v == 0
	case string:
		return v == "" || v == "[]"
	}
	return false
}

// cell is what a stored value shows as, for a column of the kind given.
func (g *GristDoc) cell(kind string, v any) string {
	kind, of, _ := strings.Cut(kind, ":")
	switch v := v.(type) {
	case nil:
		return ""
	case string:
		// Lists are kept as JSON.
		var list []any
		if (kind == "ChoiceList" || kind == "RefList" || kind == "Attachments") && strings.HasPrefix(v, "[") && json.Unmarshal([]byte(v), &list) == nil {
			parts := make([]string, 0, len(list))
			for _, item := range list {
				n, number := item.(float64)
				switch {
				case number && kind == "RefList":
					parts = append(parts, fmt.Sprintf("%s[%s]", of, numberText(n)))
				case number && kind == "Attachments":
					name := g.files[int64(n)]
					if name == "" {
						name = "attachment " + numberText(n)
					}
					parts = append(parts, name)
				default:
					parts = append(parts, g.plain(item))
				}
			}
			return oneLine(strings.Join(parts, ", "))
		}
		return oneLine(v)
	case []byte:
		object, err := unmarshalPython(v)
		if err != nil {
			return fmt.Sprintf("[blob %s bytes]", grouped(len(v)))
		}
		return oneLine(g.object(object))
	case int64:
		return g.number(kind, of, float64(v))
	case float64:
		return g.number(kind, of, v)
	}
	return ""
}

func (g *GristDoc) number(kind, of string, n float64) string {
	switch kind {
	case "Bool":
		if n == 0 {
			return "FALSE"
		} else if n == 1 {
			return "TRUE"
		}
	case "Date":
		return g.when(n, "UTC", false)
	case "DateTime":
		return g.when(n, of, true)
	case "Ref":
		if n == 0 {
			return ""
		}
		return fmt.Sprintf("%s[%s]", of, numberText(n))
	}
	return numberText(n)
}

// when writes seconds since 1970 as a date, with the time of day in the zone
// named, or the document's. Where the zone is not known to the machine the
// time is given in UTC, and says so.
func (g *GristDoc) when(seconds float64, zone string, clock bool) string {
	if math.IsNaN(seconds) || math.Abs(seconds) > 1e11 {
		return numberText(seconds)
	}
	if zone == "" {
		zone = g.zone
	}
	if zone == "" {
		zone = "UTC"
	}
	loc, known := g.zones[zone]
	if !known {
		loc, _ = time.LoadLocation(zone)
		if g.zones == nil {
			g.zones = map[string]*time.Location{}
		}
		g.zones[zone] = loc
	}
	suffix := ""
	if loc == nil {
		loc, suffix = time.UTC, " UTC"
	}
	whole := math.Floor(seconds)
	t := time.Unix(int64(whole), 0).In(loc)
	switch {
	case !clock:
		return t.Format("2006-01-02")
	case t.Second() != 0:
		return t.Format("2006-01-02 15:04:05") + suffix
	}
	return t.Format("2006-01-02 15:04") + suffix
}

// object is what a value Grist marshalled shows as. A list whose first item
// is a letter is one of Grist's own kinds: an error, a date, a reference.
func (g *GristDoc) object(v any) string {
	list, ok := v.([]any)
	if !ok || len(list) == 0 {
		return g.plain(v)
	}
	code, _ := list[0].(string)
	rest := list[1:]
	text := func(i int) string {
		if i < len(rest) {
			return g.plain(rest[i])
		}
		return ""
	}
	number := func(i int) (float64, bool) {
		if i < len(rest) {
			switch n := rest[i].(type) {
			case int64:
				return float64(n), true
			case float64:
				return n, true
			}
		}
		return 0, false
	}
	switch code {
	case "E":
		if name := text(0); name != "" {
			return "#" + name
		}
		return "#ERROR"
	case "d":
		if n, ok := number(0); ok {
			return g.when(n, "UTC", false)
		}
	case "D":
		if n, ok := number(0); ok {
			return g.when(n, text(1), true)
		}
	case "R":
		if n, ok := number(1); ok {
			return fmt.Sprintf("%s[%s]", text(0), numberText(n))
		}
	case "r":
		return fmt.Sprintf("%s%s", text(0), text(1))
	case "L":
		parts := make([]string, len(rest))
		for i := range rest {
			parts[i] = g.object(rest[i])
		}
		return strings.Join(parts, ", ")
	case "O":
		return text(0)
	case "U":
		return text(0)
	case "P":
		return "#PENDING"
	case "C":
		return "#CENSORED"
	case "S":
		return ""
	}
	return g.plain(v)
}

// plain writes a value with nothing of Grist's about it.
func (g *GristDoc) plain(v any) string {
	switch v := v.(type) {
	case nil:
		return ""
	case string:
		return v
	case bool:
		if v {
			return "TRUE"
		}
		return "FALSE"
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return numberText(v)
	case []any:
		parts := make([]string, len(v))
		for i := range v {
			parts[i] = g.object(v[i])
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case [][2]any:
		parts := make([]string, len(v))
		for i, pair := range v {
			parts[i] = g.plain(pair[0]) + ": " + g.object(pair[1])
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return ""
}

// fields describes the document.
func (g *GristDoc) fields() []Field {
	var tables []string
	for i, name := range g.Names() {
		size := ""
		if sheet, err := g.Sheet(i); err == nil {
			size = fmt.Sprintf(" (%s × %s)", plural(len(sheet.Rows)-1+sheet.MoreRows, "row"), plural(sheet.Columns+sheet.MoreColumns, "column"))
		}
		tables = append(tables, name+size)
	}
	version := ""
	if g.version > 0 {
		version = strconv.FormatInt(g.version, 10)
	}
	return Section("Grist document",
		Field{"Format", "Grist document"},
		Field{"Tables", strings.Join(tables, ", ")},
		Field{"Time zone", g.zone},
		Field{"Schema version", version},
		Field{"Not shown", "row ids, sort positions, and Grist's helper columns"})
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func gristText(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	}
	return ""
}

func gristFloat(v any) float64 {
	switch v := v.(type) {
	case int64:
		return float64(v)
	case float64:
		return v
	}
	return 0
}

func gristInt(v any) int64 {
	switch v := v.(type) {
	case int64:
		return v
	case float64:
		if !math.IsNaN(v) && math.Abs(v) < 1<<62 {
			return int64(v)
		}
	}
	return 0
}
