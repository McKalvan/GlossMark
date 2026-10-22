// @gloss implements gmi:GraphBrowser scope=file

package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mckalvan/glossmark/internal/browse"
	"github.com/mckalvan/glossmark/internal/review"
)

// browser serves the graph browser (internal/browse) from a review
// session's current build.
type browser struct{ e *review.Engine }

func (b browser) Lookup(ref string) (*browse.Node, error) {
	st, err := b.e.State()
	if err != nil {
		return nil, err
	}
	return browse.Lookup(st.Graph, st.Report, ref)
}

func (b browser) Index() ([]browse.Neighbor, error) {
	st, err := b.e.State()
	if err != nil {
		return nil, err
	}
	return browse.Index(st.Graph), nil
}

// reviewNode is gloss review node [ref]: one node and its edges, or the
// index without a ref.
func reviewNode(e *review.Engine, args []string, format string) int {
	b := browser{e}
	if len(args) == 0 {
		idx, err := b.Index()
		if err != nil {
			return fail(err)
		}
		if format == "json" {
			return printJSON(idx)
		}
		for _, nb := range idx {
			fmt.Printf("%-9s %s  %s\n", nb.Type, nb.Ref, nb.Kind)
		}
		return 0
	}
	n, err := b.Lookup(args[0])
	if err != nil {
		return fail(err)
	}
	if format == "json" {
		return printJSON(n)
	}
	printNode(os.Stdout, n)
	return 0
}

func printNode(w io.Writer, n *browse.Node) {
	fmt.Fprintf(w, "%s  %s %s %s\n", n.Ref, n.Type, n.Kind, n.Tier)
	if n.Doc != "" {
		fmt.Fprintln(w, n.Doc)
	}
	if n.Path != "" {
		fmt.Fprintf(w, "at %s:%d\n", n.Path, n.Line)
	}
	for _, f := range n.Findings {
		fmt.Fprintf(w, "! %s: %s\n", f.Kind, f.Msg)
	}
	for _, dir := range []struct {
		arrow string
		gs    []browse.Group
	}{{"→", n.Out}, {"←", n.In}} {
		for _, g := range dir.gs {
			var refs []string
			for _, nb := range g.Nodes {
				r := nb.Ref
				if nb.Proposed {
					r += "?"
				}
				refs = append(refs, r)
			}
			fmt.Fprintf(w, "%s %s: %s\n", dir.arrow, g.Relation, strings.Join(refs, ", "))
		}
	}
}
