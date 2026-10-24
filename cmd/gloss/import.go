// @gloss implements gmi:VocabularyImporter scope=file

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mckalvan/glossmark/internal/importers"
)

func init() {
	register(command{"import", "import an existing vocabulary into an owned namespace: --from markdown-glossary|enums <file> --namespace ns (re-runnable)", cmdImport})
}

func cmdImport(args []string) int {
	fs, root := newFlags("import")
	from := fs.String("from", "", "source format: markdown-glossary | enums")
	ns := fs.String("namespace", "", "owned namespace prefix to write into")
	dry := fs.Bool("dry-run", false, "report what would change and write nothing")
	values := fs.Bool("values", false, "enums: also add each value as a concept that is-a its enum")
	instances := fs.String("instances", "", "enums: comma-separated enums whose values are catalogue instances (plans, regions, currencies): the enum is imported, its values are reported and never become concepts")
	valueNames := fs.String("value-names", "bare", "enums --values: bare (Active; <Enum><Value> only on a collision) or prefixed (LoanStatusActive)")
	if err := parseFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() != 1 || *from == "" || *ns == "" {
		fmt.Fprintln(os.Stderr, "usage: gloss import --from markdown-glossary|enums <file> --namespace <ns> [--dry-run] [--values] [--instances Enum,...]")
		return 2
	}
	p, err := buildGraph(*root, false)
	if err != nil {
		return fail(err)
	}
	path, err := ontologyPath(p, *ns)
	if err != nil {
		return fail(err)
	}
	b, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return fail(err)
	}
	var entries []importers.Entry
	var insts []importers.Instance
	switch *from {
	case "markdown-glossary", "markdown", "glossary":
		entries = importers.ParseGlossary(string(b))
	case "enums", "enum":
		if *valueNames != "bare" && *valueNames != "prefixed" {
			return fail(fmt.Errorf("--value-names %q: use bare or prefixed", *valueNames))
		}
		var names []string
		for _, n := range strings.Split(*instances, ",") {
			if n = strings.TrimSpace(n); n != "" {
				names = append(names, n)
			}
		}
		entries, insts, err = importers.ParseEnumsWith(b, importers.EnumOptions{Values: *values, Prefixed: *valueNames == "prefixed", Instances: names})
		if err != nil {
			return fail(fmt.Errorf("%s: %w", fs.Arg(0), err))
		}
		for _, n := range names {
			found := false
			for _, e := range entries {
				found = found || e.Name == importers.Pascal(n)
			}
			if !found {
				return fail(fmt.Errorf("--instances %s: %s has no such enum", n, fs.Arg(0)))
			}
		}
	default:
		return fail(fmt.Errorf("--from %q: use markdown-glossary or enums", *from))
	}
	rec, err := importers.OpenRecord(p.WS.StatePath(importers.RecordName))
	if err != nil {
		return fail(err)
	}
	res, err := importers.ApplyRecorded(path, *ns, entries, reviewer(p.WS.Root), *dry, rec)
	if err != nil {
		return fail(err)
	}
	if !*dry {
		if err := rec.Save(); err != nil {
			return fail(err)
		}
	}
	verb := "imported"
	if *dry {
		verb = "would import"
	}
	fmt.Printf("gloss import: %s %d entries from %s into %s: %d added, %d updated (doc or aliases), %d unchanged, %d skipped\n",
		verb, len(entries), fs.Arg(0), relOrAbs(p.WS.Root, path), len(res.Added), len(res.Updated), len(res.Unchanged), len(res.Skipped))
	if len(res.Added) > 0 {
		var added []string
		for _, n := range res.Added {
			if k := res.Kinds[n]; k != "" {
				n += " (" + k + ")"
			}
			added = append(added, n)
		}
		fmt.Printf("  added: %s\n", strings.Join(added, ", "))
	}
	if len(res.Kept) > 0 {
		fmt.Printf("  kept: %s (edited since the last import; the source has not changed)\n", strings.Join(res.Kept, ", "))
	}
	if len(res.Updated) > 0 {
		fmt.Printf("  updated: %s\n", strings.Join(res.Updated, ", "))
	}
	for _, s := range res.Skipped {
		fmt.Printf("  skipped: %s\n", s)
	}
	if len(insts) > 0 {
		by := map[string][]string{}
		var enums []string
		for _, in := range insts {
			if by[in.Enum] == nil {
				enums = append(enums, in.Enum)
			}
			by[in.Enum] = append(by[in.Enum], in.Name)
		}
		for _, en := range enums {
			fmt.Printf("  instances of %s (not written; instances have no file format yet): %s\n", en, strings.Join(by[en], ", "))
		}
	}
	if !*dry && len(res.Added)+len(res.Updated) > 0 {
		fmt.Printf("gloss import: kinds are guessed where the source has none (a **Kind:** line sets one); review them, then gloss check. %s/%s records what was imported; commit it\n", ".glossmark", importers.RecordName)
	}
	return 0
}
