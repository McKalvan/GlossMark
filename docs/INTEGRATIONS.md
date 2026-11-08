<!-- @gloss documents gmi:BackstageExport, gmi:BackstagePlugin scope=file -->
# Portal and docs-site integrations

GlossMark's graph can be shown where a team already looks instead of in its own page. The seam is one command and one directory of JSON:

```sh
gloss export --backstage [-o dir] [--source-url tpl]        # one repository: dir/<boundary-slug>.json + dir/landscape.json (default dir: glossmark/)
gloss landscape --backstage dir ../repo-a ../repo-b         # several: the same, over the union, each link through its own repository's source-url
```

Each `<boundary-slug>.json` is one boundary (a `.gloss.yaml` manifest's directory: a service, a package, a deployment) with everything a component page shows: its label, level, owner and criticality; the concepts it realizes, holds, presents, hosts, configures, produces, consumes, publishes or subscribes to, each with kind, criticality, status, whether the manifest declares it and the code derives it, the code under the boundary and the tests behind it, linked through `source-url`; its coverage; its open drift findings linked to the code or manifest line; and the last review decisions under it with the gate class each had. `landscape.json` indexes every boundary with its drift summary. The format is `format: 1`; [`schemas/backstage-export.schema.json`](../schemas/backstage-export.schema.json) describes both and [ADR 0053](adr/0053-portal-export.md) says why they are shaped so. The slug is the boundary CURIE lower-cased with every run of other characters one hyphen (`claimb:assessment` → `claimb-assessment`).

Run it in CI after `gloss build` and publish the directory as static files (a bucket, GitHub Pages, under the docs site). Nothing calls `gloss` at page time.

## Backstage

[`integrations/backstage`](../integrations/backstage/README.md) is a standard Backstage frontend plugin (`@backstage/core-plugin-api`, `@backstage/plugin-catalog-react`, Backstage's own components) with:

- `EntityGlossMarkContent`: the **GlossMark tab** of a Component or System. It fetches the entity's document from the `glossmark.dev/export-url` annotation, or from `glossmark.baseUrl` in `app-config.yaml` plus the `glossmark.dev/boundary` annotation (a CURIE or slug), and shows the boundary card, coverage and open-finding counts, one table per relation of concepts with criticality badges, code and tests, the open drift findings and the last decisions. `isGlossMarkAvailable` is the `if=` for the route.
- `GlossMarkLandscapePage`: the **landscape page** at a route of your choosing, listing every boundary across repositories from `landscape.json`, sorted by worst finding then criticality, with repository, owner, concept count, drift summary, coverage and a link to each document, and the context map when `gloss landscape` wrote it.

```yaml
# catalog-info.yaml                                  # app-config.yaml
metadata:                                            glossmark:
  annotations:                                         baseUrl: https://docs.example.com/glossmark
    glossmark.dev/boundary: claimb:assessment
```

The README has the install steps, the `EntityPage.tsx` and `App.tsx` snippets, and `yarn tsc`, `yarn lint`, `yarn test`. The package is not published to npm; link it from your Backstage repository's `plugins/`. The reverse direction of the same join, deriving a manifest's `owner` from `catalog-info.yaml`'s `spec.owner`, is [issue #207](https://github.com/McKalvan/GlossMark/issues/207).

## MkDocs and Docusaurus (not built yet)

The docs-site plugins the issue asks for, an embedded view or slice and a browsable glossary page regenerated on every build, are a follow-up. They will read the same directory (`landscape.json` for the index, the boundary documents for each page, `gloss export --status` or `gloss publish` for the glossary) so that a site build needs the files and no `gloss` binary. Until then, `gloss view --inline` writes a self-contained `viewer.html` a docs site can iframe, and `gloss export --status -o status.json` gives a glossary generator each concept's definition and status.
