// Package store exports the graph to SQLite: nodes, edges and provenance
// tables plus recursive-CTE views for the is-a and part-of closures, so any
// SQL client can query the ontology.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"

	_ "modernc.org/sqlite"

	"github.com/mckalvan/glossmark/internal/graph"
)

const schema = `
CREATE TABLE prefixes (prefix TEXT PRIMARY KEY, iri TEXT NOT NULL);
CREATE TABLE nodes (
  id TEXT PRIMARY KEY, curie TEXT, node TEXT NOT NULL, ns TEXT, tier TEXT, kind TEXT, label TEXT, doc TEXT,
  published_as TEXT, internal INTEGER NOT NULL DEFAULT 0, deprecated INTEGER NOT NULL DEFAULT 0, replaced_by TEXT,
  value TEXT, imported INTEGER NOT NULL DEFAULT 0, version TEXT, owner TEXT, path TEXT, symbol TEXT, scope TEXT,
  fingerprint TEXT, boundary TEXT, file TEXT, line INTEGER, json TEXT NOT NULL
);
CREATE INDEX nodes_curie ON nodes(curie);
CREATE INDEX nodes_node ON nodes(node);
CREATE TABLE edges (
  id INTEGER PRIMARY KEY, s TEXT NOT NULL, p TEXT NOT NULL, o TEXT NOT NULL, source TEXT NOT NULL,
  path TEXT, line INTEGER, scope TEXT, symbol TEXT, fingerprint TEXT, proposed INTEGER NOT NULL DEFAULT 0,
  conf REAL, marker_id TEXT, note TEXT, actor TEXT, rule TEXT, via TEXT, raw TEXT, json TEXT NOT NULL
);
CREATE INDEX edges_s ON edges(s, p);
CREATE INDEX edges_o ON edges(o, p);
CREATE INDEX edges_p ON edges(p);
-- is-a closure: (sub, super, depth)
CREATE VIEW is_a_closure AS
WITH RECURSIVE c(sub, super, depth) AS (
  SELECT s, o, 1 FROM edges WHERE p = 'is-a'
  UNION
  SELECT c.sub, e.o, c.depth + 1 FROM c JOIN edges e ON e.s = c.super AND e.p = 'is-a' WHERE c.depth < 64
) SELECT * FROM c;
-- part-of closure across concepts, boundaries and artifacts
CREATE VIEW part_of_closure AS
WITH RECURSIVE c(part, whole, depth) AS (
  SELECT s, o, 1 FROM edges WHERE p = 'part-of'
  UNION
  SELECT c.part, e.o, c.depth + 1 FROM c JOIN edges e ON e.s = c.whole AND e.p = 'part-of' WHERE c.depth < 64
) SELECT * FROM c;
-- realizes closure (impl → core, following realizes and published-as)
CREATE VIEW realizes_closure AS
WITH RECURSIVE c(impl, core, depth) AS (
  SELECT s, o, 1 FROM edges WHERE p IN ('realizes','published-as') AND source <> 'derived'
  UNION
  SELECT c.impl, e.o, c.depth + 1 FROM c JOIN edges e ON e.s = c.core AND e.p IN ('realizes','published-as') AND e.source <> 'derived' WHERE c.depth < 64
) SELECT * FROM c;
-- convenience: concept → artifacts linking to it (directly)
CREATE VIEW concept_artifacts AS
SELECT e.o AS concept, n.curie AS concept_curie, e.s AS artifact, a.path, a.symbol, e.p AS relation, e.proposed, e.marker_id
FROM edges e JOIN nodes a ON a.id = e.s AND a.node = 'artifact' JOIN nodes n ON n.id = e.o;
CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT);
`

// Export writes the graph into a new SQLite file at path (replacing any existing file).
func Export(g *graph.Graph, path string) error {
	os.Remove(path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("schema: %w", err)
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for p, iri := range g.Prefixes {
		if _, err := tx.Exec("INSERT INTO prefixes VALUES (?, ?)", p, iri); err != nil {
			return err
		}
	}
	ins, err := tx.Prepare(`INSERT INTO nodes (id, curie, node, ns, tier, kind, label, doc, published_as, internal, deprecated, replaced_by, value, imported, version, owner, path, symbol, scope, fingerprint, boundary, file, line, json)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	for _, n := range g.Nodes() {
		js, _ := json.Marshal(n)
		var val any
		if n.Value != nil {
			val = fmt.Sprint(n.Value)
		}
		sym, scope, fp := "", "", ""
		if n.Anchor != nil {
			sym, scope, fp = n.Anchor.Symbol, n.Anchor.Scope, n.Anchor.Fingerprint
		}
		if _, err := ins.Exec(n.ID, n.CURIE, string(n.NodeType), n.NS, n.Tier, n.Kind, n.Label, n.Doc, n.Published, b2i(n.Internal), b2i(n.Deprecated), n.ReplacedBy, val, b2i(n.Imported), n.Version, n.Owner, n.Path, sym, scope, fp, n.Boundary, n.File, n.Line, string(js)); err != nil {
			return fmt.Errorf("node %s: %w", n.ID, err)
		}
	}
	ine, err := tx.Prepare(`INSERT INTO edges (s, p, o, source, path, line, scope, symbol, fingerprint, proposed, conf, marker_id, note, actor, rule, via, raw, json)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	for _, e := range g.Edges() {
		js, _ := json.Marshal(e)
		p := e.Prov
		if p == nil {
			p = &graph.Prov{}
		}
		var conf any
		if p.Conf != nil {
			conf = *p.Conf
		}
		via, _ := json.Marshal(p.Via)
		if _, err := ine.Exec(e.S, e.P, e.O, e.Source, p.Path, p.Line, p.Scope, p.Symbol, p.Fingerprint, b2i(p.Proposed), conf, p.ID, p.Note, p.Actor, p.Rule, string(via), p.Raw, string(js)); err != nil {
			return fmt.Errorf("edge: %w", err)
		}
	}
	tx.Exec("INSERT INTO meta VALUES ('generator', ?)", g.Generator)
	return tx.Commit()
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
