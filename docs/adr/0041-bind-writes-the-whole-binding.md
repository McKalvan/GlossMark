<!-- @gloss documents gm:ValueBinding scope=file -->
# ADR 0041: `gloss bind` writes the whole binding, and HARD changes name their owner

Status: accepted, 2026-10-28; built 2026-10-28 (#272).

## Context

Value binding is the feature a newcomer understands first: "the auto-approval
limit is 5000 EUR; changing that constant is a governed event, not a one-line
diff." Until now it took hand-written YAML (`value:`, `value-from:`,
`published:`) or a marker whose relation and scope had to be known, and the
gate reported a drifted value without saying whose decision it was. The demo
in [docs/DEMO.md](../DEMO.md) needed one command and one answer to "who
decides".

## Decision

1. **One command writes everything the binding needs.** `gloss bind
   <path>:<symbol> --as <ns:Name>` resolves the symbol with the same resolver
   the extractor uses (`extract.ResolveValueFrom`, so what `bind` reads is
   what `build` will bind), reads the literal the line assigns
   (`extract.ValueLiteral`; `--value` overrides it with the business's
   wording), writes the ontology entry through `ontoedit` (new entry, or the
   fields on an existing one, in the file's own style), inserts the marker
   above the line in the file's comment style, and builds with the ledger so
   the line is the baseline from that moment. A dry run (`-n`) prints the
   three writes.

2. **Both forms are written: `value-from:` in the ontology and
   `publishes-value` in the code.** They bind the same line, which the
   extractor records once (bindings are keyed by location). The ontology
   reader learns where the value lives without opening the code; the code
   reader learns the line is governed without opening the ontology. A file
   type with no comment syntax (JSON) gets `value-from:` alone, and
   `--no-marker` chooses that for any file.

3. **`bind` is a person's act.** The entry is written accepted, not
   `proposed: true`, like `gloss accept`, because publishing a value is the
   decision an agent is not allowed to make (`gloss propose` remains the
   agent's path for a concept). An implementation concept must say what rule
   it is: `--publish-as <core>` (which implies `--realizes`), `--realizes
   <core>`, or `--internal`; the error names the choice. A core concept takes
   neither.

4. **A HARD change names its owner.** `gate.Change` carries `Owner`: the
   `owner:` of the manifest the changed path is under, else the `owner:` of
   the subject's namespace. The text and Markdown reports append `owner X` to
   the HARD tag; JSON carries the field. Only HARD changes carry it, since
   only those wait on a person.

## Consequences

- A drifted value in the gate reads `[HIGH, HARD: published value drifted: the
  bound code changed and value: did not, owner claims-decisioning]`.
- Running `bind` again on a bound concept rewrites `value:` in place and moves
  the baseline: the way to record that the business agreed to the new value.
- `examples/value-binding` and `examples/value-binding/demo.sh` are the
  shipped demo; CI runs the script so it keeps working.
