# Example: a constant becomes a governed business rule

The smallest workspace that shows value binding: one Python service whose
settings hold `AUTO_APPROVE_LIMIT_EUR = 5_000`, a core tier that names the rule
(`claim:AutoApprovalLimit`, criticality high) and an implementation tier with
nothing bound yet. It is a cut-down `assessment` service from the
[claims-events showcase](https://github.com/McKalvan/glossmark-examples).

`demo.sh` copies this directory into a temporary git repository and runs the
sixty-second demo in [docs/DEMO.md](../../docs/DEMO.md): `gloss bind` the
constant, change it, watch `gloss drift` and `gloss diff` catch it, then bind
the agreed value. From the repository root:

```sh
examples/value-binding/demo.sh
```

Before the bind, `gloss check` passes and `gloss drift` reports one gap: the
manifest says `services/assessment` realizes `claim:AutoApprovalLimit`, but no
line of code is linked to it. The bind closes it.
