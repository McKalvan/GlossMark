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

// ImportsDir holds the pinned copy of every locked import, under StateDir.
const ImportsDir = "imports"

// Resolved describes an import as the loader found it.
type Resolved struct {
	Prefix  string
	Version string
	Range   string
	Source  string
	Hash    string
	Path    string
	// Pinned: resolved from the copy gloss lock vendored, not from Source.
	// Upstream is then what Source resolves to now (nil when unreachable),
	// which gloss lock --update moves to.
	Pinned   bool
	Upstream *Resolved
}

// PinPath is where gloss lock vendors an import:
// .glossmark/imports/<prefix>@<version>.gloss.yaml.
func PinPath(root, prefix, version string) string {
	return filepath.Join(root, StateDir, ImportsDir, prefix+"@"+orNone(version)+".gloss.yaml")
}

// pinned returns the vendored copy of an import when the lock pins it from
// the same source and the copy still matches the locked hash; else nil.
func pinned(root, prefix string, im *model.ImportDecl, lock *Lock) *model.Ontology {
	if lock == nil {
		return nil
	}
	e, ok := lock.Imports[prefix]
	if !ok || e.Hash == "" || e.Source != im.Source {
		return nil
	}
	path := PinPath(root, prefix, e.Version)
	if hashFile(path) != e.Hash {
		return nil
	}
	o, err := model.LoadOntology(path)
	if err != nil {
		return nil
	}
	return o
}

// RangeProblem says why a resolved import may not be locked under its
// range: no version, a non-semver one, or one outside the range. It returns
// the diagnostic code (GM105 or GM106) and message, or "" when it satisfies.
func RangeProblem(r *Resolved) (string, string) {
	if r.Range == "" {
		return "", ""
	}
	c, err := model.ParseConstraint(r.Range)
	if err != nil || c.IsAny() {
		return "", ""
	}
	if r.Version == "" {
		return "GM105", fmt.Sprintf("imports.%s: requires %s but the imported ontology declares no version; add version: to its %s.gloss.yaml", r.Prefix, r.Range, r.Prefix)
	}
	v, err := model.ParseSemver(r.Version)
	if err != nil {
		return "GM105", fmt.Sprintf("imports.%s: imported version %q is not semver", r.Prefix, r.Version)
	}
	if !c.Satisfies(v) {
		return "GM106", fmt.Sprintf("imports.%s: requires %s but the resolved ontology is %s (%s)", r.Prefix, r.Range, r.Version, r.Source)
	}
	return "", ""
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

// WriteLock writes the lockfile from the resolved imports and vendors each
// resolved ontology to its PinPath, removing copies nothing pins any more.
// Every command then resolves the import from that copy while its hash
// matches the lock, so the lock is a pin, not only a tripwire.
func WriteLock(root string, resolved map[string]*Resolved) error {
	l := Lock{Format: version.LockFormat, Imports: map[string]*LockEntry{}}
	keep := map[string]bool{}
	for p, r := range resolved {
		b, err := os.ReadFile(r.Path)
		if err != nil {
			return fmt.Errorf("imports.%s: %w", p, err)
		}
		dst := PinPath(root, p, r.Version)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			return err
		}
		keep[dst] = true
		h := sha256.Sum256(b)
		l.Imports[p] = &LockEntry{Version: r.Version, Range: r.Range, Source: r.Source, Hash: "sha256:" + hex.EncodeToString(h[:])}
	}
	old, _ := filepath.Glob(filepath.Join(root, StateDir, ImportsDir, "*@*.gloss.yaml"))
	for _, f := range old {
		if !keep[f] {
			os.Remove(f)
		}
	}
	b, err := yaml.Marshal(&l)
	if err != nil {
		return err
	}
	head := "# glossmark.lock: resolved import versions, written by gloss lock. Commit it with .glossmark/imports/.\n"
	return os.WriteFile(filepath.Join(root, LockName), append([]byte(head), b...), 0o644)
}

// SortedPrefixes returns the prefixes of resolved imports in order, so every
// listing of them is stable.
func SortedPrefixes(resolved map[string]*Resolved) []string {
	out := make([]string, 0, len(resolved))
	for p := range resolved {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
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
	prefixes := SortedPrefixes(ws.Resolved)
	for _, p := range prefixes {
		switch code, msg := RangeProblem(ws.Resolved[p]); code {
		case "GM105":
			out = append(out, marker.Diagnostic{Path: ConfigName, Line: 1, Severity: "warning", Code: code, Msg: msg})
		case "GM106":
			out = append(out, marker.Diagnostic{Path: ConfigName, Line: 1, Severity: "error", Code: code, Msg: msg})
		}
	}
	if ws.lockErr != nil {
		out = append(out, marker.Diagnostic{Path: LockName, Line: 1, Severity: "error", Code: "GM109", Msg: ws.lockErr.Error()})
		return out
	}
	lock := ws.Lock
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
	locked := make([]string, 0, len(lock.Imports))
	for p := range lock.Imports {
		locked = append(locked, p)
	}
	sort.Strings(locked)
	for _, p := range locked {
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
