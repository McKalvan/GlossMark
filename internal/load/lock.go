// @gloss implements gmi:ImportLock scope=file

package load

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/mckalvan/glossmark/internal/marker"
	"github.com/mckalvan/glossmark/internal/model"
	"github.com/mckalvan/glossmark/internal/version"
)

// LockName is the lockfile beside glossmark.yaml.
const LockName = "glossmark.lock"

// Lock pins the resolved version and content of every import so a build is
// reproducible and an upstream change is noticed.
type Lock struct {
	Format  int                   `yaml:"format"`
	Imports map[string]*LockEntry `yaml:"imports"`
}

// LockEntry is one pinned import.
type LockEntry struct {
	Version string `yaml:"version"`
	Range   string `yaml:"range,omitempty"`
	Source  string `yaml:"source,omitempty"`
	Hash    string `yaml:"hash"` // sha256 of the resolved ontology file
}

// Resolved describes an import as the loader found it.
type Resolved struct {
	Prefix  string
	Version string
	Range   string
	Source  string
	Hash    string
	Path    string
}

// ReadLock loads glossmark.lock; a missing file returns nil, nil.
func ReadLock(root string) (*Lock, error) {
	b, err := os.ReadFile(filepath.Join(root, LockName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var l Lock
	if err := yaml.Unmarshal(b, &l); err != nil {
		return nil, fmt.Errorf("%s: %w", LockName, err)
	}
	if l.Format > version.LockFormat {
		return nil, fmt.Errorf("%s: format %d was written by a newer gloss; this release reads format %d", LockName, l.Format, version.LockFormat)
	}
	if l.Imports == nil {
		l.Imports = map[string]*LockEntry{}
	}
	return &l, nil
}

// WriteLock writes the lockfile from the resolved imports.
func WriteLock(root string, resolved map[string]*Resolved) error {
	l := Lock{Format: version.LockFormat, Imports: map[string]*LockEntry{}}
	for p, r := range resolved {
		l.Imports[p] = &LockEntry{Version: r.Version, Range: r.Range, Source: r.Source, Hash: r.Hash}
	}
	b, err := yaml.Marshal(&l)
	if err != nil {
		return err
	}
	head := "# glossmark.lock: resolved import versions, written by gloss lock. Commit it.\n"
	return os.WriteFile(filepath.Join(root, LockName), append([]byte(head), b...), 0o644)
}

// hashFile returns sha256:<hex> of a file.
func hashFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}

// verifyImports checks version ranges and the lockfile, returning diagnostics.
func verifyImports(ws *Workspace) []marker.Diagnostic {
	var out []marker.Diagnostic
	prefixes := make([]string, 0, len(ws.Resolved))
	for p := range ws.Resolved {
		prefixes = append(prefixes, p)
	}
	sort.Strings(prefixes)
	for _, p := range prefixes {
		r := ws.Resolved[p]
		if r.Range == "" {
			continue
		}
		c, err := model.ParseConstraint(r.Range)
		if err != nil || c.IsAny() {
			continue
		}
		if r.Version == "" {
			out = append(out, marker.Diagnostic{Path: ConfigName, Line: 1, Severity: "warning", Code: "GM105",
				Msg: fmt.Sprintf("imports.%s: requires %s but the imported ontology declares no version; add version: to its %s.gloss.yaml", p, r.Range, p)})
			continue
		}
		v, err := model.ParseSemver(r.Version)
		if err != nil {
			out = append(out, marker.Diagnostic{Path: ConfigName, Line: 1, Severity: "warning", Code: "GM105", Msg: fmt.Sprintf("imports.%s: imported version %q is not semver", p, r.Version)})
			continue
		}
		if !c.Satisfies(v) {
			out = append(out, marker.Diagnostic{Path: ConfigName, Line: 1, Severity: "error", Code: "GM106",
				Msg: fmt.Sprintf("imports.%s: requires %s but the resolved ontology is %s (%s)", p, r.Range, r.Version, r.Source)})
		}
	}
	lock, err := ReadLock(ws.Root)
	if err != nil {
		out = append(out, marker.Diagnostic{Path: LockName, Line: 1, Severity: "error", Code: "GM109", Msg: err.Error()})
		return out
	}
	ws.Lock = lock
	if len(ws.Resolved) == 0 {
		return out
	}
	if lock == nil {
		out = append(out, marker.Diagnostic{Path: ConfigName, Line: 1, Severity: "warning", Code: "GM108",
			Msg: fmt.Sprintf("no %s: run gloss lock to pin the resolved import versions", LockName)})
		return out
	}
	for _, p := range prefixes {
		r := ws.Resolved[p]
		e, ok := lock.Imports[p]
		if !ok {
			out = append(out, marker.Diagnostic{Path: LockName, Line: 1, Severity: "warning", Code: "GM107", Msg: fmt.Sprintf("imports.%s is not in %s; run gloss lock", p, LockName)})
			continue
		}
		switch {
		case e.Version != r.Version:
			out = append(out, marker.Diagnostic{Path: LockName, Line: 1, Severity: "error", Code: "GM107",
				Msg: fmt.Sprintf("imports.%s: locked at %s but resolved %s; review the upstream change, then run gloss lock", p, e.Version, orNone(r.Version))})
		case e.Hash != "" && e.Hash != r.Hash:
			out = append(out, marker.Diagnostic{Path: LockName, Line: 1, Severity: "error", Code: "GM107",
				Msg: fmt.Sprintf("imports.%s: ontology content changed since it was locked at %s (same version number); the upstream should bump its version. Run gloss lock to accept", p, e.Version)})
		}
	}
	for p := range lock.Imports {
		if _, ok := ws.Resolved[p]; !ok {
			out = append(out, marker.Diagnostic{Path: LockName, Line: 1, Severity: "warning", Code: "GM107", Msg: fmt.Sprintf("%s pins %s, which is no longer imported; run gloss lock", LockName, p)})
		}
	}
	return out
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
