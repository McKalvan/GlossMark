// @gloss implements gmi:LandscapeAssembler scope=file

package load

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/mckalvan/glossmark/internal/model"
)

// Merge unions several workspaces into one landscape workspace: namespaces,
// manifests, vocabulary and files are combined; each repo's files are
// prefixed with the repo name so paths stay unique. The first workspace's
// views and policy apply.
func Merge(wss []*Workspace) (*Workspace, error) {
	if len(wss) == 0 {
		return nil, fmt.Errorf("no workspaces to merge")
	}
	out := &Workspace{Root: filepath.Dir(wss[0].Root), Config: &model.Config{Owns: map[string]*model.NamespaceDecl{}, Imports: map[string]*model.ImportDecl{}, Prefixes: map[string]string{}},
		Ontologies: map[string]*model.Ontology{}, Owned: map[string]bool{}, Imported: map[string]*model.ImportDecl{}, Vocab: model.NewVocabulary(), Prefixes: map[string]string{}}
	out.Config.Policy = wss[0].Config.Policy
	out.Config.Views = wss[0].Config.Views
	out.Views = wss[0].Views
	// Two prefixes bound to one namespace IRI would merge two namespaces
	// into one by IRI, silently (siblings initialised with the same base).
	byIRI := map[string]string{}
	bind := func(repo, prefix, iri string) error {
		if iri == "" {
			return nil
		}
		if ex, ok := byIRI[iri]; ok && ex != prefix {
			return fmt.Errorf("namespaces %s: and %s: both resolve to %s (in %s); give each namespace its own IRI", ex, prefix, iri, repo)
		}
		byIRI[iri] = prefix
		return nil
	}
	for _, ws := range wss {
		name := filepath.Base(ws.Root)
		for _, p := range sortedKeys(ws.Config.Owns) {
			if err := bind(name, p, ws.Config.Owns[p].IRI); err != nil {
				return nil, err
			}
		}
		for _, p := range sortedKeys(ws.Config.Imports) {
			if err := bind(name, p, ws.Config.Imports[p].IRI); err != nil {
				return nil, err
			}
		}
	}
	for _, ws := range wss {
		name := filepath.Base(ws.Root)
		for p, iri := range ws.Prefixes {
			if ex, ok := out.Prefixes[p]; ok && ex != iri {
				return nil, fmt.Errorf("prefix %s bound to %s in one repo and %s in another", p, ex, iri)
			}
			out.Prefixes[p] = iri
		}
		for p, d := range ws.Config.Owns {
			if _, ok := out.Config.Owns[p]; ok {
				return nil, fmt.Errorf("namespace %s: is owned by two repositories", p)
			}
			out.Config.Owns[p] = d
			out.Owned[p] = true
			out.Ontologies[p] = ws.Ontologies[p]
		}
		for p, d := range ws.Config.Imports {
			if !out.Owned[p] {
				if _, ok := out.Ontologies[p]; !ok {
					out.Ontologies[p] = ws.Ontologies[p]
					out.Imported[p] = d
					out.Config.Imports[p] = d
				}
			}
		}
		for _, m := range ws.Manifests {
			mm := *m
			mm.Path = name + "/" + m.Path
			if m.Dir == "" {
				mm.Dir = name
			} else {
				mm.Dir = name + "/" + m.Dir
			}
			out.Manifests = append(out.Manifests, &mm)
		}
		for _, f := range ws.Files {
			out.Files = append(out.Files, name+"/"+f)
		}
		out.Diags = append(out.Diags, ws.Diags...)
	}
	// An import that is owned by another member becomes owned in the landscape.
	for p := range out.Imported {
		if out.Owned[p] {
			delete(out.Imported, p)
			delete(out.Config.Imports, p)
		}
	}
	for p, o := range out.Ontologies {
		names := make([]string, 0, len(o.Relations))
		for n := range o.Relations {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			out.Vocab.Override(n, o.Relations[n])
		}
		_ = p
	}
	sort.Strings(out.Files)
	return out, nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
