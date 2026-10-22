// @gloss implements gmi:ReviewEngine scope=file

package review

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mckalvan/glossmark/internal/load"
)

// JournalSize is how many decisions undo can walk back.
const JournalSize = 100

// snapshot holds, before a decision, every file the decision can touch.
type snapshot struct {
	root   string
	before map[string][]byte // abs path → content; nil when the file did not exist
}

// takeSnapshot reads the files a decision can change: the state files, the
// config, the owned ontologies, every file with a marker and the file a new
// link goes into.
func takeSnapshot(st *State, d Decision) (*snapshot, error) {
	ws := st.WS
	paths := map[string]bool{
		stateFile(ws, "ledger.jsonl"): true, stateFile(ws, "graph.jsonl"): true, stateFile(ws, "context.jsonld"): true,
		stateFile(ws, RecommendationsFile): true, filepath.Join(ws.Root, load.ConfigName): true,
	}
	for _, ns := range ws.Config.Owns {
		if ns != nil && ns.File != "" {
			paths[ws.Abs(ns.File)] = true
		}
	}
	for _, fm := range st.Markers {
		paths[ws.Abs(fm.Path)] = true
	}
	if d.Path != "" {
		paths[ws.Abs(d.Path)] = true
	}
	s := &snapshot{root: ws.Root, before: map[string][]byte{}}
	for p := range paths {
		b, err := os.ReadFile(p)
		switch {
		case err == nil:
			s.before[p] = b
		case os.IsNotExist(err):
			s.before[p] = nil
		default:
			return nil, err
		}
	}
	return s, nil
}

type journalFile struct {
	Path    string `json:"path"` // repo-relative
	Existed bool   `json:"existed"`
	Before  []byte `json:"before,omitempty"`
	After   string `json:"after"` // sha256 of the content the decision left; "" when it removed the file
}

type journalEntry struct {
	ID    string        `json:"id"`
	At    string        `json:"at"`
	Actor string        `json:"actor"`
	Desc  string        `json:"desc"`
	Files []journalFile `json:"files"`
}

// journal records the files that changed since the snapshot and returns the
// entry id, or "" when nothing changed.
func (s *snapshot) journal(root, desc, actor string, now time.Time) (string, error) {
	var files []journalFile
	for p, before := range s.before {
		after, err := os.ReadFile(p)
		gone := os.IsNotExist(err)
		if err != nil && !gone {
			return "", err
		}
		if (before == nil) == gone && bytes.Equal(before, after) {
			continue
		}
		r, _ := filepath.Rel(s.root, p)
		jf := journalFile{Path: filepath.ToSlash(r), Existed: before != nil, Before: before}
		if !gone {
			jf.After = hash(after)
		}
		files = append(files, jf)
	}
	if len(files) == 0 {
		return "", nil
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	dir, err := journalDir(root)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	id := fmt.Sprintf("%020d", now.UnixNano())
	b, err := json.Marshal(journalEntry{ID: id, At: now.Format(time.RFC3339), Actor: actor, Desc: desc, Files: files})
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, id+".json"), b, 0o644); err != nil {
		return "", err
	}
	entries, _ := journalEntries(dir)
	for len(entries) > JournalSize {
		os.Remove(filepath.Join(dir, entries[0]))
		entries = entries[1:]
	}
	return id, nil
}

// Undo reverts the newest journaled decision. It refuses when a file has
// changed again since, unless force, because restoring would lose that edit.
func (e *Engine) Undo(force bool) (string, error) {
	dir, err := journalDir(e.Root)
	if err != nil {
		return "", err
	}
	names, err := journalEntries(dir)
	if err != nil || len(names) == 0 {
		return "", errors.New("nothing to undo")
	}
	name := names[len(names)-1]
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return "", err
	}
	var je journalEntry
	if err := json.Unmarshal(b, &je); err != nil {
		return "", fmt.Errorf("undo journal %s: %w", name, err)
	}
	var changed []string
	for _, f := range je.Files {
		cur, err := os.ReadFile(filepath.Join(e.Root, filepath.FromSlash(f.Path)))
		now := ""
		if err == nil {
			now = hash(cur)
		}
		if now != f.After {
			changed = append(changed, f.Path)
		}
	}
	if len(changed) > 0 && !force {
		return "", fmt.Errorf("cannot undo %q: %s changed since; undoing would discard that (force to undo anyway)", je.Desc, strings.Join(changed, ", "))
	}
	for _, f := range je.Files {
		p := filepath.Join(e.Root, filepath.FromSlash(f.Path))
		if !f.Existed {
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				return "", err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(p, f.Before, 0o644); err != nil {
			return "", err
		}
	}
	if err := os.Remove(filepath.Join(dir, name)); err != nil {
		return "", err
	}
	e.Invalidate()
	return fmt.Sprintf("undid %s (%d files restored)", je.Desc, len(je.Files)), nil
}

// UndoDepth is how many decisions the journal can walk back.
func (e *Engine) UndoDepth() int {
	dir, err := journalDir(e.Root)
	if err != nil {
		return 0
	}
	names, _ := journalEntries(dir)
	return len(names)
}

func journalEntries(dir string) ([]string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// journalDir is <git dir>/glossmark/undo, so the journal stays local to the
// clone (and to the worktree), or the user's cache directory outside git.
func journalDir(root string) (string, error) {
	if out, err := exec.Command("git", "-C", root, "rev-parse", "--absolute-git-dir").Output(); err == nil {
		if d := strings.TrimSpace(string(out)); d != "" {
			return filepath.Join(d, "glossmark", "undo"), nil
		}
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	abs, _ := filepath.Abs(root)
	return filepath.Join(cache, "glossmark", "undo", hash([]byte(abs))[:16]), nil
}

func hash(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
