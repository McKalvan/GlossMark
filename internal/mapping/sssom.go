// @gloss implements gmi:MappingSets scope=file
// @gloss represents gm:MappingSet scope=file
// @gloss represents gm:Mapping scope=file

// Package mapping reads and writes SSSOM/TSV mapping sets (ADR 0025): a
// standard exchange format for equivalences between concepts of different
// namespaces, whose review fields (reviewer_id, review_date,
// predicate_modifier: Not) carry a mapping's review state in the file itself.
//
// Only standard SSSOM slots are read and written. Unknown columns and
// metadata keys are discarded, as the spec requires of an implementation
// without extension support; GlossMark keeps its own state in the ledger.
package mapping

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Builtin are the prefixes SSSOM treats as built in: they may be omitted from
// curie_map and, when present, must point here.
var Builtin = map[string]string{
	"owl":    "http://www.w3.org/2002/07/owl#",
	"rdf":    "http://www.w3.org/1999/02/22-rdf-syntax-ns#",
	"rdfs":   "http://www.w3.org/2000/01/rdf-schema#",
	"semapv": "https://w3id.org/semapv/vocab/",
	"skos":   "http://www.w3.org/2004/02/skos/core#",
	"sssom":  "https://w3id.org/sssom/",
	"xsd":    "http://www.w3.org/2001/XMLSchema#",
	"linkml": "https://w3id.org/linkml/",
}

// Columns is the Mapping class's slot order: the canonical column order.
var Columns = []string{"record_id", "subject_id", "subject_label", "subject_category", "predicate_id", "predicate_label", "predicate_modifier",
	"object_id", "object_label", "object_category", "mapping_justification", "author_id", "author_label", "reviewer_id", "reviewer_label",
	"creator_id", "creator_label", "license", "subject_type", "subject_source", "subject_source_version", "object_type", "object_source",
	"object_source_version", "predicate_type", "mapping_provider", "mapping_source", "mapping_cardinality", "cardinality_scope", "mapping_tool",
	"mapping_tool_id", "mapping_tool_version", "mapping_date", "publication_date", "review_date", "confidence", "reviewer_agreement",
	"curation_rule", "curation_rule_text", "subject_match_field", "object_match_field", "match_string", "subject_preprocessing",
	"object_preprocessing", "similarity_score", "similarity_measure", "see_also", "issue_tracker_item", "derived_from", "other", "comment"}

var columnRank = func() map[string]int {
	m := map[string]int{}
	for i, c := range Columns {
		m[c] = i
	}
	return m
}()

// Propagatable are the set-level slots that apply to every mapping in the set.
var Propagatable = map[string]bool{"curation_rule": true, "curation_rule_text": true, "cardinality_scope": true, "mapping_date": true,
	"mapping_provider": true, "mapping_tool": true, "mapping_tool_version": true, "object_match_field": true, "object_preprocessing": true,
	"object_source": true, "object_source_version": true, "object_type": true, "subject_match_field": true, "subject_preprocessing": true,
	"subject_source": true, "subject_source_version": true, "subject_type": true, "predicate_type": true, "similarity_measure": true}

// Problem is a parse finding, reported by the loader as a diagnostic.
type Problem struct {
	Line     int
	Severity string // error | warning
	Code     string
	Msg      string
}

// Set is one SSSOM mapping set.
type Set struct {
	// Name is the key under mappings: in glossmark.yaml; Path is the TSV file,
	// repo-relative; Owner and Scope come from the same entry.
	Name, Path, Owner, Scope string

	ID, Version, License, Title string
	Creators                    []string
	CurieMap                    map[string]string

	// meta is the raw metadata block (without '#'), kept verbatim on rewrite;
	// External is the .sssom.yml file when the set uses external metadata.
	meta     []string
	External string
	// propagated are the set-level slot values copied into every row on read
	// and therefore never written back into rows.
	propagated map[string]string

	Rows []*Row
	// Problems found while reading.
	Problems []Problem
}

// Row is one mapping record. Cells holds every standard slot with a value,
// propagated values included; Line is the row's first physical line.
type Row struct {
	Line  int
	Cells map[string]string
}

// Get returns a slot's value ("" when absent).
func (r *Row) Get(slot string) string { return r.Cells[slot] }

// Set assigns a slot ("" deletes it).
func (r *Row) Set(slot, v string) {
	if v == "" {
		delete(r.Cells, slot)
		return
	}
	r.Cells[slot] = v
}

// Multi returns a multi-valued slot's values.
func (r *Row) Multi(slot string) []string { return SplitMulti(r.Cells[slot]) }

// Negative reports whether the row negates its predicate (predicate_modifier: Not).
func (r *Row) Negative() bool { return r.Cells["predicate_modifier"] == "Not" }

// Reviewed reports whether someone reviewed and confirmed the row.
func (r *Row) Reviewed() bool { return r.Cells["reviewer_id"] != "" || r.Cells["reviewer_label"] != "" }

// Confidence returns the confidence in [0,1], or -1 when absent or invalid.
func (r *Row) Confidence() float64 {
	if v := r.Cells["confidence"]; v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f <= 1 {
			return f
		}
	}
	return -1
}

// Load reads a set from an SSSOM/TSV file (embedded or external metadata).
func Load(path string) (*Set, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := &Set{CurieMap: map[string]string{}, propagated: map[string]string{}}
	if bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}) {
		s.problem(1, "error", "GM700", "file starts with a byte order mark; SSSOM/TSV forbids it")
		b = b[3:]
	}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	lines := strings.SplitAfter(text, "\n")
	// Metadata block: leading lines that start with '#'.
	n := 0
	for n < len(lines) && strings.HasPrefix(lines[n], "#") {
		n++
	}
	var metaText string
	if n > 0 {
		raw := make([]string, n)
		for i := 0; i < n; i++ {
			raw[i] = strings.TrimSuffix(strings.TrimPrefix(lines[i], "#"), "\n")
		}
		// Every line may carry the same number of leading spaces after '#'.
		indent := -1
		for _, l := range raw {
			if strings.TrimSpace(l) == "" {
				continue
			}
			k := len(l) - len(strings.TrimLeft(l, " "))
			if indent < 0 || k < indent {
				indent = k
			}
		}
		if indent < 0 {
			indent = 0
		}
		for i, l := range raw {
			if len(l) >= indent {
				raw[i] = l[indent:]
			}
		}
		s.meta = raw
		metaText = strings.Join(raw, "\n")
	} else {
		ext := strings.TrimSuffix(path, filepath.Ext(path)) + ".yml"
		if !strings.HasSuffix(strings.TrimSuffix(path, filepath.Ext(path)), ".sssom") {
			ext = strings.TrimSuffix(path, filepath.Ext(path)) + ".sssom.yml"
		}
		mb, err := os.ReadFile(ext)
		if err != nil {
			s.problem(1, "error", "GM700", fmt.Sprintf("no metadata block and no external metadata file %s", filepath.Base(ext)))
		} else {
			s.External = ext
			metaText = string(mb)
		}
	}
	s.parseMeta(metaText)
	s.parseBody(strings.Join(lines[n:], ""), n+1)
	return s, nil
}

func (s *Set) problem(line int, sev, code, msg string) {
	s.Problems = append(s.Problems, Problem{Line: line, Severity: sev, Code: code, Msg: msg})
}

func (s *Set) parseMeta(text string) {
	var m map[string]any
	if strings.TrimSpace(text) != "" {
		if err := yaml.Unmarshal([]byte(text), &m); err != nil {
			s.problem(1, "error", "GM700", "metadata block: "+err.Error())
			return
		}
	}
	str := func(k string) string {
		if v, ok := m[k]; ok && v != nil {
			return fmt.Sprint(v)
		}
		return ""
	}
	s.ID, s.Version, s.License, s.Title = str("mapping_set_id"), str("mapping_set_version"), str("license"), str("mapping_set_title")
	switch v := m["creator_id"].(type) {
	case string:
		s.Creators = []string{v}
	case []any:
		for _, x := range v {
			s.Creators = append(s.Creators, fmt.Sprint(x))
		}
	}
	if cm, ok := m["curie_map"].(map[string]any); ok {
		for k, v := range cm {
			s.CurieMap[k] = fmt.Sprint(v)
			if b, ok := Builtin[k]; ok && b != fmt.Sprint(v) {
				s.problem(1, "error", "GM700", fmt.Sprintf("curie_map: built-in prefix %s must be %s", k, b))
			}
		}
	}
	if s.ID == "" {
		s.problem(1, "error", "GM700", "metadata: mapping_set_id is required")
	}
	if s.License == "" {
		s.problem(1, "error", "GM700", "metadata: license is required")
	}
	for k, v := range m {
		if Propagatable[k] && v != nil {
			s.propagated[k] = fmt.Sprint(v)
		}
	}
}

// parseBody reads the TSV section; first is the physical line of its header.
func (s *Set) parseBody(text string, first int) {
	records, starts := splitRecords(text)
	if len(records) == 0 {
		s.problem(first, "error", "GM700", "no header line")
		return
	}
	header := records[0]
	var cols []string
	for _, h := range header {
		h = strings.TrimSpace(h)
		if _, ok := columnRank[h]; !ok {
			s.problem(first, "warning", "GM700", fmt.Sprintf("unknown column %q discarded (SSSOM readers without extension support must drop it)", h))
			h = ""
		}
		cols = append(cols, h)
	}
	if !contains(cols, "subject_id") || !contains(cols, "predicate_id") || !contains(cols, "object_id") || !contains(cols, "mapping_justification") {
		s.problem(first, "error", "GM700", "header must include subject_id, predicate_id, object_id and mapping_justification")
	}
	for i, rec := range records[1:] {
		line := first + starts[i+1]
		if len(rec) == 1 && strings.TrimSpace(rec[0]) == "" {
			continue
		}
		if len(rec) != len(cols) {
			s.problem(line, "error", "GM700", fmt.Sprintf("row has %d values, header has %d columns", len(rec), len(cols)))
		}
		r := &Row{Line: line, Cells: map[string]string{}}
		for j, v := range rec {
			if j < len(cols) && cols[j] != "" && v != "" {
				r.Cells[cols[j]] = v
			}
		}
		s.Rows = append(s.Rows, r)
	}
	// Propagation: a set-level value applies to every row, unless a row has its own.
	for k, v := range s.propagated {
		own := false
		for _, r := range s.Rows {
			if r.Cells[k] != "" {
				own = true
			}
		}
		if own {
			delete(s.propagated, k)
			continue
		}
		for _, r := range s.Rows {
			r.Cells[k] = v
		}
	}
	for _, r := range s.Rows {
		if r.Get("subject_id") == "" || r.Get("predicate_id") == "" || r.Get("object_id") == "" {
			s.problem(r.Line, "error", "GM700", "row needs subject_id, predicate_id and object_id")
		}
		if r.Get("mapping_justification") == "" {
			s.problem(r.Line, "error", "GM700", "row needs mapping_justification (semapv:ManualMappingCuration, semapv:LexicalMatching, ...)")
		} else if !strings.HasPrefix(r.Get("mapping_justification"), "semapv:") {
			s.problem(r.Line, "warning", "GM700", fmt.Sprintf("mapping_justification %q is not a semapv term", r.Get("mapping_justification")))
		}
		if m := r.Get("predicate_modifier"); m != "" && m != "Not" {
			s.problem(r.Line, "error", "GM700", fmt.Sprintf("predicate_modifier must be Not, got %q", m))
		}
		if v := r.Get("confidence"); v != "" && r.Confidence() < 0 {
			s.problem(r.Line, "error", "GM700", fmt.Sprintf("confidence must be a number in [0,1], got %q", v))
		}
		if r.Get("review_date") != "" && !r.Reviewed() {
			s.problem(r.Line, "error", "GM700", "review_date requires reviewer_id or reviewer_label")
		}
	}
}

// splitRecords parses tab-separated records with RFC 4180 double-quote rules:
// a quoted value may hold tabs, newlines and doubled quotes. starts gives each
// record's 0-based line offset in text.
func splitRecords(text string) ([][]string, []int) {
	var recs [][]string
	var starts []int
	var rec []string
	var cur strings.Builder
	inQ, atStart := false, true
	line, recLine := 0, 0
	flushField := func() { rec = append(rec, cur.String()); cur.Reset(); atStart = true }
	flushRec := func() {
		flushField()
		recs = append(recs, rec)
		starts = append(starts, recLine)
		rec = nil
	}
	rs := []rune(text)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case inQ:
			if c == '"' {
				if i+1 < len(rs) && rs[i+1] == '"' {
					cur.WriteRune('"')
					i++
				} else {
					inQ = false
				}
			} else {
				if c == '\n' {
					line++
				}
				cur.WriteRune(c)
			}
		case c == '"' && atStart:
			inQ, atStart = true, false
		case c == '\t':
			flushField()
		case c == '\n':
			flushRec()
			line++
			recLine = line
		default:
			atStart = false
			cur.WriteRune(c)
		}
	}
	if cur.Len() > 0 || len(rec) > 0 {
		flushRec()
	}
	return recs, starts
}

// SplitMulti splits a |-separated multi-valued cell, honouring \| and \\.
func SplitMulti(v string) []string {
	if v == "" {
		return nil
	}
	var out []string
	var cur strings.Builder
	for i := 0; i < len(v); i++ {
		switch {
		case v[i] == '\\' && i+1 < len(v) && (v[i+1] == '|' || v[i+1] == '\\'):
			cur.WriteByte(v[i+1])
			i++
		case v[i] == '|':
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(v[i])
		}
	}
	return append(out, cur.String())
}

// JoinMulti is the inverse of SplitMulti.
func JoinMulti(vs []string) string {
	esc := make([]string, len(vs))
	for i, v := range vs {
		esc[i] = strings.ReplaceAll(strings.ReplaceAll(v, `\`, `\\`), "|", `\|`)
	}
	return strings.Join(esc, "|")
}

// Expand turns a CURIE into an IRI using the set's curie_map, then the
// built-ins, then fallback (the workspace's prefixes). declared is false when
// the prefix came from fallback: other SSSOM tools reject such a file.
func (s *Set) Expand(curie string, fallback map[string]string) (iri string, declared, ok bool) {
	i := strings.Index(curie, ":")
	if i <= 0 {
		return "", false, false
	}
	p := curie[:i]
	if base, found := s.CurieMap[p]; found {
		return base + curie[i+1:], true, true
	}
	if base, found := Builtin[p]; found {
		return base + curie[i+1:], true, true
	}
	if base, found := fallback[p]; found {
		return base + curie[i+1:], false, true
	}
	return "", false, false
}

// Save writes the set back in canonical SSSOM/TSV: the metadata block as it
// was (or as generated for a new set), then the rows sorted on every slot in
// column order, with only the columns that hold a value.
func (s *Set) Save(path string) error {
	var sb strings.Builder
	if s.External == "" {
		meta := s.meta
		if len(meta) == 0 {
			meta = s.generatedMeta()
		}
		for _, l := range meta {
			if l == "" {
				sb.WriteString("#\n")
				continue
			}
			sb.WriteString("#" + l + "\n")
		}
	}
	used := map[string]bool{}
	for _, r := range s.Rows {
		for k, v := range r.Cells {
			if v != "" && s.propagated[k] == "" {
				used[k] = true
			}
		}
	}
	for _, c := range []string{"subject_id", "predicate_id", "object_id", "mapping_justification"} {
		used[c] = true // the minimal header, even for an empty set
	}
	var cols []string
	for _, c := range Columns {
		if used[c] {
			cols = append(cols, c)
		}
	}
	rows := append([]*Row{}, s.Rows...)
	sort.SliceStable(rows, func(i, j int) bool {
		for _, c := range Columns {
			a, b := rows[i].Cells[c], rows[j].Cells[c]
			if a != b {
				return a < b
			}
		}
		return false
	})
	sb.WriteString(strings.Join(cols, "\t") + "\n")
	for _, r := range rows {
		vals := make([]string, len(cols))
		for i, c := range cols {
			vals[i] = quote(r.Cells[c])
		}
		sb.WriteString(strings.Join(vals, "\t") + "\n")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

func quote(v string) string {
	if strings.ContainsAny(v, "\t\n\"") {
		return `"` + strings.ReplaceAll(v, `"`, `""`) + `"`
	}
	return v
}

// generatedMeta renders a new set's metadata block in canonical form.
func (s *Set) generatedMeta() []string {
	var out []string
	add := func(k, v string) {
		if v != "" {
			out = append(out, k+": "+yamlScalar(v))
		}
	}
	keys := make([]string, 0, len(s.CurieMap))
	for k := range s.CurieMap {
		if _, b := Builtin[k]; !b {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		out = append(out, "curie_map:")
		for _, k := range keys {
			out = append(out, "  "+k+": "+yamlScalar(s.CurieMap[k]))
		}
	}
	add("mapping_set_id", s.ID)
	add("mapping_set_version", s.Version)
	add("mapping_set_title", s.Title)
	if len(s.Creators) > 0 {
		out = append(out, "creator_id:")
		for _, c := range s.Creators {
			out = append(out, "  - "+yamlScalar(c))
		}
	}
	add("license", s.License)
	return out
}

func yamlScalar(v string) string {
	b, _ := yaml.Marshal(v)
	return strings.TrimSpace(string(b))
}

// SetMetaValue rewrites (or adds) one top-level scalar key in the metadata
// block, keeping every other line; used to bump mapping_set_version and to
// add a prefix to curie_map.
func (s *Set) SetMetaValue(key, value string) {
	if len(s.meta) == 0 {
		s.meta = s.generatedMeta()
	}
	for i, l := range s.meta {
		if strings.HasPrefix(l, key+":") {
			s.meta[i] = key + ": " + yamlScalar(value)
			return
		}
	}
	s.meta = append(s.meta, key+": "+yamlScalar(value))
}

// AddCurie declares a prefix in curie_map (metadata block) when missing.
func (s *Set) AddCurie(prefix, iri string) {
	if _, ok := Builtin[prefix]; ok {
		return
	}
	if s.CurieMap[prefix] == iri {
		return
	}
	s.CurieMap[prefix] = iri
	if len(s.meta) == 0 {
		return // generatedMeta renders the map
	}
	for i, l := range s.meta {
		if strings.HasPrefix(l, "curie_map:") {
			rest := strings.TrimSpace(strings.TrimPrefix(l, "curie_map:"))
			if strings.HasPrefix(rest, "{") {
				// Flow mapping: rewrite the line.
				keys := make([]string, 0, len(s.CurieMap))
				for k := range s.CurieMap {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				var parts []string
				for _, k := range keys {
					parts = append(parts, k+": "+yamlScalar(s.CurieMap[k]))
				}
				s.meta[i] = "curie_map: { " + strings.Join(parts, ", ") + " }"
				return
			}
			// Block mapping: rewrite the indented lines under the key, sorted.
			j := i + 1
			ind := "  "
			for j < len(s.meta) && strings.HasPrefix(s.meta[j], " ") {
				ind = s.meta[j][:len(s.meta[j])-len(strings.TrimLeft(s.meta[j], " "))]
				j++
			}
			keys := make([]string, 0, len(s.CurieMap))
			for k := range s.CurieMap {
				if _, b := Builtin[k]; !b {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)
			var block []string
			for _, k := range keys {
				block = append(block, ind+k+": "+yamlScalar(s.CurieMap[k]))
			}
			tail := append([]string{}, s.meta[j:]...)
			s.meta = append(append(s.meta[:i+1], block...), tail...)
			return
		}
	}
	s.meta = append([]string{"curie_map:", "  " + prefix + ": " + yamlScalar(iri)}, s.meta...)
}

// ReadLines returns a file's lines (for callers that report context).
func ReadLines(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		out = append(out, sc.Text())
	}
	return out
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// SaveNew writes a new set: a generated metadata block and an empty body.
func (s *Set) SaveNew(path string) error {
	s.meta = s.generatedMeta()
	return s.Save(path)
}
