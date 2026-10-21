// @gloss implements gmi:MappingSets scope=file

package load

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"

	"github.com/mckalvan/glossmark/internal/mapping"
	"github.com/mckalvan/glossmark/internal/marker"
)

// loadMappings reads every mapping set declared under mappings:, reporting
// parse problems as diagnostics at the set file's lines (GM700).
func (ws *Workspace) loadMappings() {
	names := make([]string, 0, len(ws.Config.Mappings))
	for n := range ws.Config.Mappings {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		decl := ws.Config.Mappings[name]
		rel := filepath.ToSlash(filepath.Clean(decl.File))
		s, err := mapping.Load(ws.Abs(rel))
		if err != nil {
			msg := err.Error()
			if errors.Is(err, fs.ErrNotExist) {
				msg = fmt.Sprintf("mapping set %s: %s not found (create it with gloss map init %s)", name, rel, name)
			}
			ws.Diags = append(ws.Diags, marker.Diagnostic{Path: ConfigName, Line: 1, Severity: "error", Code: "GM700", Msg: msg})
			continue
		}
		s.Name, s.Path, s.Owner, s.Scope = name, rel, decl.Owner, decl.Scope
		for _, p := range s.Problems {
			ws.Diags = append(ws.Diags, marker.Diagnostic{Path: rel, Line: p.Line, Severity: p.Severity, Code: p.Code, Msg: p.Msg})
		}
		ws.MappingSets = append(ws.MappingSets, s)
	}
}

// MappingSet returns the set with a name, or nil.
func (ws *Workspace) MappingSet(name string) *mapping.Set {
	for _, s := range ws.MappingSets {
		if s.Name == name {
			return s
		}
	}
	return nil
}
