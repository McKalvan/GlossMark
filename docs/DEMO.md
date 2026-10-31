<!-- @gloss documents gm:ValueBinding, gmi:BindCommand scope=file -->
# Sixty seconds: a constant becomes a governed business rule

The auto-approval limit is 5000 EUR. Today it is a line in `settings.py`, and
raising it is a one-line diff that any reviewer can wave through. After one
command it is a published business rule: the ontology says what the value is,
the code line is bound to it, and changing the line alone fails the build and
goes HARD in the pull-request gate, with the owner named.

Run it: `examples/value-binding/demo.sh` from a checkout (it builds `gloss`,
copies [`examples/value-binding`](../examples/value-binding/README.md) into a
temporary git repository and runs the steps below; under a minute once Go has
built). Or follow along by hand.

## 0. A plain constant

```python
# services/assessment/assessment/settings.py
# Claims at or below this estimated loss, with no fraud hold and complete evidence,
# are approved without an adjuster.
AUTO_APPROVE_LIMIT_EUR = 5_000
```

The core tier already names the rule in the business's words, with its
criticality; nothing in the code is linked to it yet:

```yaml
# ontology/claim.gloss.yaml
  AutoApprovalLimit:
    kind: constraint
    constrains: [ClaimAssessment]
    doc: "The largest estimated loss the rules may approve without an adjuster."
    criticality: high
```

## 1. Bind it

```sh
gloss bind services/assessment/assessment/settings.py:AUTO_APPROVE_LIMIT_EUR \
    --as tide:AutoApprovalRule --publish-as claim:AutoApprovalLimit --value "5000 EUR"
```

```
gloss bind: bind tide:AutoApprovalRule = "5000 EUR", bound to services/assessment/assessment/settings.py:6 (AUTO_APPROVE_LIMIT_EUR = 5_000)
  ontology/tide.gloss.yaml: adds tide:AutoApprovalRule (constraint, realizes claim:AutoApprovalLimit, published as claim:AutoApprovalLimit, value "5000 EUR")
  services/assessment/assessment/settings.py:6: # @gloss publishes-value .AutoApprovalRule scope=line
  baseline sha256:d36502ca376dda4b recorded in .glossmark/ledger.jsonl; commit it with the ontology and the code
  a change to services/assessment/assessment/settings.py:7 without a change to value: is now drift (gloss drift) and HARD (gloss diff); gloss check confirms the entry
```

Three things were written, and all three belong in the same commit:

- The ontology entry, in the implementation tier, formatted like its
  neighbours. `value:` is what the business sees (`--value`; without it, the
  literal the line assigns, `5_000` read as `5000`). `value-from:` is the
  line it comes from, found by symbol, so the binding survives edits above
  it. `published:` puts the rule on the seam into the business view, which is
  what makes a change to it HARD.
  ```yaml
    AutoApprovalRule:
      kind: constraint
      realizes: claim:AutoApprovalLimit
      published: { as: claim:AutoApprovalLimit }
      value: "5000 EUR"
      value-from: { path: services/assessment/assessment/settings.py, symbol: AUTO_APPROVE_LIMIT_EUR }
      doc: "AUTO_APPROVE_LIMIT_EUR in services/assessment/assessment/settings.py."
  ```
- The marker above the line, in the file's comment style, so whoever edits
  the constant sees that it is governed:
  ```python
  # @gloss publishes-value .AutoApprovalRule scope=line
  AUTO_APPROVE_LIMIT_EUR = 5_000
  ```
- The baseline: a `value:tide:AutoApprovalRule` row in `.glossmark/ledger.jsonl`
  with the line's fingerprint, which is what `gloss drift` compares against.

`gloss check` passes, and the gap the manifest had (`services/assessment`
declares that it realizes `claim:AutoApprovalLimit`, but nothing under it was
linked) closes: a `publishes-value` marker counts as `implements`.

## 2. Change the constant alone

```sh
sed -i 's/= 5_000/= 7_500/' services/assessment/assessment/settings.py
gloss drift
```

```
≠ Published values drifted (the bound code changed, the value did not)
  → Update value: in the ontology to match the code, or confirm the code with gloss accept --value.
  tide:AutoApprovalRule publishes "5000 EUR", bound to services/assessment/assessment/settings.py:7, which changed from "AUTO_APPROVE_LIMIT_EUR = 5_000" to "AUTO_APPROVE_LIMIT_EUR = 7_500"; update value: in the ontology, or confirm the line as is with gloss accept --value --force tide:AutoApprovalRule
...
gloss drift: failed: 1 drifted value
```

In a pull request the same change is HARD, and the boundary's `owner:` is
named on it:

```sh
gloss diff --base HEAD
```

```
Ontology impact — ws   base HEAD   gate HARD   criticality high
≠ published value of tide:AutoApprovalRule drifted: services/assessment/assessment/settings.py:7 changed from "AUTO_APPROVE_LIMIT_EUR = 5_000" to "AUTO_APPROVE_LIMIT_EUR = 7_500" while value: stays 5000 EUR   services/assessment/assessment/settings.py   [HIGH, HARD: published value drifted: the bound code changed and value: did not, owner claims-decisioning]
...
Criticality touched: tide:AutoApprovalRule high (via claim:AutoApprovalLimit)
gloss diff: HARD changes need a human decision
```

`gloss impact --base HEAD` adds what the line touches: the core rule, the
constrained code, the test that verifies it, and that both concepts are on
the published seam. The GitHub Action posts the same report as the PR
comment.

## 3. Decide

Either the business agrees to the new value, or the code goes back. Agreeing
is one more bind, which rewrites `value:` in place and moves the baseline:

```sh
gloss bind services/assessment/assessment/settings.py:AUTO_APPROVE_LIMIT_EUR --as tide:AutoApprovalRule --value "7500 EUR"
gloss accept --stale services/assessment/assessment/settings.py   # the marker's line changed: re-confirm the link
gloss drift      # clean
gloss diff --base HEAD
```

```
! published tide:AutoApprovalRule value 5000 EUR → 7500 EUR    ontology/tide.gloss.yaml:10   [HIGH, HARD: edit to ontology; business view affected: published concept, owner claims-engineering]
! namespace tide: changes require a PATCH bump but version went 0.1.0 → 0.1.0 (none)   ontology/tide.gloss.yaml   [HARD: semver: bump the namespace version; ...]
```

The diff is still HARD, now for the right reason: an edit to a published
concept, which the namespace owner decides in the pull request (and a bump
of `version:` in `tide.gloss.yaml`, since importers pin it). Code, ontology
and baseline change together, in one reviewable diff. (`gloss accept --value tide:AutoApprovalRule` confirms the
code without changing `value:`, for when the line still holds the published
value and only its wording moved; see [ADVANCED.md](ADVANCED.md).)

## Where it fits

- `gloss bind` works on any language GlossMark comments (Python, Go, Kotlin,
  Java, TypeScript, Terraform, ...) and on a key path in a YAML, TOML or JSON
  file (`config/ratelimit.yaml:policies.claims-write.per_minute`; JSON takes
  no comment, so the ontology's `value-from:` alone binds it). `--internal`
  binds an engineering-private setting without publishing it; `-n` prints
  what would be written. `gloss bind -h` has the flags.
- The underlying fields are `value`, `value-from` and `published` in
  [FILE-FORMATS.md](FILE-FORMATS.md) and the `publishes-value` relation in
  [ADVANCED.md](ADVANCED.md); [ADR 0017](adr/0017-published-values-implement.md)
  says why a published value counts as an implementation and
  [ADR 0047](adr/0047-bind-writes-the-whole-binding.md) why `bind` writes
  both forms and names the owner.
- Next: [QUICKSTART.md](QUICKSTART.md) for adopting GlossMark on your own
  repository, where `gloss init` scaffolds the files this example has by hand.
