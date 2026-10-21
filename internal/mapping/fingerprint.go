// @gloss implements gmi:MappingSets scope=file
// @gloss publishes gm:MappingStale scope=file

package mapping

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/mckalvan/glossmark/internal/graph"
	"github.com/mckalvan/glossmark/internal/model"
)

// ConceptFingerprint hashes what a mapping relies on about a concept: its
// kind, label and definition, its declared is-a parents, its publication and
// its deprecation (ADR 0025 §4). A mapping reviewed against one fingerprint
// is stale when either endpoint's fingerprint changes. It is "" when the
// concept is not loaded (a name under prefixes: only), which tracks nothing.
func ConceptFingerprint(g *graph.Graph, iri string) string {
	n := g.Node(iri)
	if n == nil || n.NodeType != model.NodeConcept {
		return ""
	}
	return fingerprintOf(Facts(g, iri))
}

// Facts lists the fingerprinted facts of a concept as "key=value" lines, so a
// diff can say which one changed.
func Facts(g *graph.Graph, iri string) []string {
	n := g.Node(iri)
	if n == nil {
		return nil
	}
	var parents []string
	for _, e := range g.Out(iri, "is-a") {
		if e.Source == graph.SourceDeclared {
			parents = append(parents, g.Curie(e.O))
		}
	}
	sort.Strings(parents)
	kind := n.Kind
	if n.KindFrom != "" {
		kind = "" // inherited: the parent list already carries it
	}
	return []string{
		"kind=" + kind,
		"label=" + n.Label,
		"doc=" + strings.TrimSpace(n.Doc),
		"is-a=" + strings.Join(parents, ","),
		"published-as=" + n.Published,
		fmt.Sprintf("deprecated=%v", n.Deprecated),
		"replaced-by=" + n.ReplacedBy,
	}
}

func fingerprintOf(facts []string) string {
	h := sha256.Sum256([]byte(strings.Join(facts, "\n")))
	return "sha256:" + hex.EncodeToString(h[:])[:16]
}

// ChangedFacts returns the keys whose values differ between two fact lists.
func ChangedFacts(before, after []string) []string {
	split := func(xs []string) map[string]string {
		m := map[string]string{}
		for _, x := range xs {
			if i := strings.Index(x, "="); i > 0 {
				m[x[:i]] = x[i+1:]
			}
		}
		return m
	}
	b, a := split(before), split(after)
	var out []string
	for _, k := range []string{"kind", "label", "doc", "is-a", "published-as", "deprecated", "replaced-by"} {
		if b[k] != a[k] {
			out = append(out, k)
		}
	}
	return out
}

// ID is the stable ledger id of a mapping: its endpoints and canonical predicate.
func ID(subject, predicate, object string) string {
	return "mapping:" + subject + "|" + predicate + "|" + object
}

// ParseID splits a mapping id into subject, predicate and object.
func ParseID(id string) (s, p, o string, ok bool) {
	parts := strings.SplitN(strings.TrimPrefix(id, "mapping:"), "|", 3)
	if !strings.HasPrefix(id, "mapping:") || len(parts) != 3 {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}
