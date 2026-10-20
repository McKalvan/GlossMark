# Worked example: a public library

Everyone knows how a library works: books come in and get catalogued, patrons
check them out, renew them and bring them back, and a returned book goes to the
next patron with a hold on it. This example marks up the software that runs that
and exercises the whole of GlossMark across two repositories that share one core.

| Repository | Owns | Imports | Boundaries |
| --- | --- | --- | --- |
| `circulation/` | `lib:` (core: what the library recognises) and `circ:` (implementation) | | `acme:library-platform` (system), `acme:circulation-svc` (container), `acme:opac` (the patron-facing catalog, a surface) |
| `holds-worker/` | `hw:` (implementation) | `lib:` `^1.0` from `circulation/`, locked in `glossmark.lock` | `acme:holds-worker` (container, part of the platform) |

```sh
go build -o gloss ./cmd/gloss                            # from the repository root
cd examples/library/circulation
../../../gloss check        # 0 errors, 1 warning: the Laptop link is outside the boundary's allowed list
../../../gloss drift        # the findings below
../../../gloss view         # .glossmark/viewer.html: boundaries are boxes, click a concept for its code, tests and findings
cd ../holds-worker && ../../../gloss check && ../../../gloss drift
cd .. && ../../gloss landscape circulation holds-worker  # both repos as one graph
```

## What the tooling makes of it

| In the code | Declared | Result |
| --- | --- | --- |
| `catalog_laptop` proposes `produces lib:Laptop` (`@gloss?`, conf 0.55) | the service `produces [lib:Media]`; a Laptop is an Item, not Media | Violation, and GM502 in `gloss check`: outside the allowed list |
| the platform produces {Book, DVD, Media, Laptop} | the system `produces [lib:Item]` | Consistent after taxonomic lift: the rollup summarizes to `{lib:Item}` |
| `renew.py` proposes `implements lib:Renewal` | the service's `realizes` leaves Renewal out | Violation: Renewal handled but not declared |
| `returns.py` proposes `implements circ:NEW:DamageCheck` | no such concept | Rogue concept: a decision request routed to `circulation-eng` |
| nothing links to `lib:InterlibraryLoan`, `circ:CoverImageCache`, `circ:SubjectSuggester` | in the ontology | Orphans (the cataloging assistant's agent file *instructs* SubjectSuggester, which is not an implementation) |
| `features/checkout.feature` verifies `lib:Checkout`; nothing verifies `lib:Return` | both implemented | Return is untested; Checkout is not |
| `apps/opac` presents `lib:Hold`; nothing in this repo implements it | | Presented but not implemented; closes in the landscape, where holds-worker represents Hold |
| `acme:library-platform` publishes `lib:HoldReady` | declared by the system | Gap here; closes in the landscape, where `hw:HoldQueue` publishes it |
| `hw:HoldQueue` subscribes to `lib:ItemReturned`; `circ:ReturnEventBus` publishes it | nothing links the repos by hand | Context-map edge `acme:holds-worker → acme:circulation-svc via lib:ItemReturned`, drawn by `gloss landscape` |
| `hw:CurbsidePickup is-a lib:HoldFulfillment` | `lib:` is imported | Allowed (the subject is `hw:`'s own) and a decision request: propose it upstream? |
| `hw:PickupClock` value `"7 days"` | published as `lib:PickupWindow` | Changing it is HARD in `gloss diff`: the business view changes |
| `LOAN_PERIOD_DAYS = 21` in `policy.py` | `circ:DefaultLoanPeriod` has `value-from` that line, published as `lib:LoanPeriod` | Editing the constant without the ontology is value drift |

The rest of the design shows up too:

- **Criticality.** `lib:Patron` is `critical` (reading records are confidential by law), `lib:Checkout`, `lib:Loan` and `lib:Hold` are `high`, `lib:Cataloging` is `medium` and the loan-period measure is `low`. The circulation service inherits `high` from Checkout, `policy.py` stays `low` because it takes the level of what it implements, and the OPAC, which only presents, inherits nothing. A proposal touching `lib:Patron` is HARD by default, while a change to `high` code asks for an owner's review (SOFT).
- **Every file type.** Python, Go, TypeScript, SQL (`CREATE TABLE` anchors), Terraform, a GitHub workflow (`deploys`), Gherkin (`verifies`), a JSON sidecar with JSON Pointers (`configures ... scope=range`), Markdown agent files (`instructs`, `may-modify` remits with globs), and a `gen/` directory excluded by `generated:`.
- **Views.** `ontology/views.yaml` defines `business` (core only, with `circ:` folded into what it realizes and the published loan period under its core name), `engineering` (both tiers, MARC parsing expanded two `part-of` levels) and a curated `loan-policy` view.
- **Manifests.** The system says `realizes: [inherit]` instead of repeating its containers, the service lists `allowed` with a glob (`circ:*`), and the OPAC declares `presents`.

`holds-worker/glossmark.yaml` imports `lib:` from a local path for this example.
In a real deployment the source is a git ref or URL, pinned to a version.
