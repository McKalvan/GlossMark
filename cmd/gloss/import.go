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
	if err := fs.Parse(flagsLast(args)); err != nil {
		return 2
	}
	if fs.NArg() != 1 || *from == "" || *ns == "" {
		fmt.Fprintln(os.Stderr, "usage: gloss import --from markdown-glossary|enums <file> --namespace <ns> [--dry-run] [--values]")
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
	switch *from {
	case "markdown-glossary", "markdown", "glossary":
		entries = importers.ParseGlossary(string(b))
	case "enums", "enum":
		entries, err = importers.ParseEnums(b, *values)
		if err != nil {
			return fail(fmt.Errorf("%s: %w", fs.Arg(0), err))
		}
	default:
		return fail(fmt.Errorf("--from %q: use markdown-glossary or enums", *from))
	}
	res, err := importers.Apply(path, entries, reviewer(p.WS.Root), *dry)
	if err != nil {
		return fail(err)
	}
	verb := "imported"
	if *dry {
		verb = "would import"
	}
	fmt.Printf("gloss import: %s %d entries from %s into %s: %d added, %d updated (doc or aliases), %d unchanged, %d skipped\n",
		verb, len(entries), fs.Arg(0), relOrAbs(p.WS.Root, path), len(res.Added), len(res.Updated), len(res.Unchanged), len(res.Skipped))
	if len(res.Added) > 0 {
		fmt.Printf("  added: %s\n", strings.Join(res.Added, ", "))
	}
	if len(res.Updated) > 0 {
		fmt.Printf("  updated: %s\n", strings.Join(res.Updated, ", "))
	}
	for _, s := range res.Skipped {
		fmt.Printf("  skipped: %s\n", s)
	}
	if !*dry && len(res.Added)+len(res.Updated) > 0 {
		fmt.Println("gloss import: kinds are guessed where the source has none; review them, then gloss check")
	}
	return 0
}
