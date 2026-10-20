// @gloss implements gmi:VocabularyImporter scope=file

package importers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/version"
)

// RecordName is the import record under .glossmark/: what each import last
// wrote per concept, so a re-run can tell a source change from a hand edit
// (ADR 0016). It is committed, like the ledger.
const RecordName = "import-record.jsonl"

// RecordFormat is the import record's format.
const RecordFormat = 1

// RecordEntry is what the last import read from the source for one concept.
type RecordEntry struct {
	NS      string   `json:"ns"`
	Name    string   `json:"name"`
	Doc     string   `json:"doc,omitempty"`
	Aliases []string `json:"aliases,omitempty"`
}

// Record is the import record of a workspace.
type Record struct {
	Path    string
	Entries map[string]*RecordEntry // ns:Name
	dirty   bool
}

type recordHeader struct {
	Type      string `json:"type"`
	Format    int    `json:"format"`
	Generator string `json:"generator"`
}

// OpenRecord reads the record at path; a missing file is an empty record.
func OpenRecord(path string) (*Record, error) {
	r := &Record{Path: path, Entries: map[string]*RecordEntry{}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return r, nil
	}
	if err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for n := 1; sc.Scan(); n++ {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if bytes.HasPrefix(line, []byte(`{"type":"header"`)) {
			var h recordHeader
			if err := json.Unmarshal(line, &h); err != nil {
				return nil, fmt.Errorf("%s:%d: %w", path, n, err)
			}
			if h.Format > RecordFormat {
				return nil, fmt.Errorf("%s: format %d was written by a newer gloss (%s); this release reads format %d", path, h.Format, h.Generator, RecordFormat)
			}
			continue
		}
		var e RecordEntry
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, n, err)
		}
		r.Entries[e.NS+":"+e.Name] = &e
	}
	return r, sc.Err()
}

func (r *Record) get(ns, name string) *RecordEntry {
	if r == nil {
		return nil
	}
	return r.Entries[ns+":"+name]
}

func (r *Record) put(ns string, e Entry) {
	if r == nil {
		return
	}
	n := &RecordEntry{NS: ns, Name: e.Name, Doc: e.Doc, Aliases: e.Aliases}
	if old := r.Entries[ns+":"+e.Name]; old != nil && old.Doc == n.Doc && strings.Join(old.Aliases, "\x00") == strings.Join(n.Aliases, "\x00") {
		return
	}
	r.Entries[ns+":"+e.Name] = n
	r.dirty = true
}

// Save writes the record, sorted by namespace and name, when it changed.
func (r *Record) Save() error {
	if r == nil || !r.dirty {
		return nil
	}
	keys := make([]string, 0, len(r.Entries))
	for k := range r.Entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	h, _ := json.Marshal(recordHeader{Type: "header", Format: RecordFormat, Generator: "gloss " + version.String()})
	buf.Write(h)
	buf.WriteByte('\n')
	for _, k := range keys {
		b, err := json.Marshal(r.Entries[k])
		if err != nil {
			return err
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(r.Path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(r.Path, buf.Bytes(), 0o644)
}
