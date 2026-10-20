// @gloss verifies gmi:SqliteExport scope=file

package main

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// TestExportFloorFiles (issue #44): under a criticality floor, an unmarked
// file is exported as a node='file' row at the floor's level, so the
// governance query lists it; files outside the floor are not.
func TestExportFloorFiles(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"glossmark.yaml":               "owns:\n  c: { iri: https://x.test/c/, tier: core }\n",
		"ontology/c.gloss.yaml":        "concepts:\n  Patient: { kind: entity }\n",
		".gloss.yaml":                  "boundary: o:clinic\nlevel: system\n",
		"db/.gloss.yaml":               "boundary: o:db\nlevel: container\npart-of: o:clinic\ncriticality: high\ncriticality-floor: true\n",
		"db/migrations/0001_init.sql":  "-- @gloss represents c:Patient\ncreate table patients (id int);\n",
		"db/migrations/0006_roles.sql": "-- grants\ngrant select on patients to reporting;\n",
		"services/api/main.go":         "package main\n\nfunc main() {}\n",
	})
	out := filepath.Join(t.TempDir(), "g.db")
	if msg, code := capture(t, "export", "--root", root, "--sqlite", out); code != 0 {
		t.Fatalf("export: %d\n%s", code, msg)
	}
	db, err := sql.Open("sqlite", out)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query("SELECT path, node, criticality, criticality_from FROM nodes WHERE node IN ('artifact','file') AND criticality IN ('high','critical') ORDER BY path")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var path, node, crit, from string
		rows.Scan(&path, &node, &crit, &from)
		got = append(got, strings.Join([]string{path, node, crit, from}, " "))
	}
	want := "db/migrations/0001_init.sql artifact high o:db\ndb/migrations/0006_roles.sql file high o:db"
	if strings.Join(got, "\n") != want {
		t.Errorf("governance query:\n%s\nwant\n%s", strings.Join(got, "\n"), want)
	}
}
