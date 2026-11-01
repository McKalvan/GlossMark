<!-- @gloss documents gmi:BackstagePlugin scope=file -->
# GlossMark for Backstage

A Backstage frontend plugin that shows a component's or system's GlossMark graph where the team already looks: an **entity tab** with the concepts the boundary realizes, holds, presents or exchanges, each with its criticality, the code and tests behind it, the boundary's coverage, its open drift findings linked to code and the last review decisions; and a **landscape page** listing every boundary across repositories with its owner, criticality and drift summary.

The plugin reads static JSON. It calls no GlossMark service: `gloss export --backstage` (one repository) or `gloss landscape --backstage` (several) writes a directory of documents, `<boundary-slug>.json` plus `landscape.json`, which you publish anywhere Backstage can fetch from (a static bucket, GitHub Pages, the docs site, a path under the Backstage app). The format is `format: 1`, described by [`schemas/backstage-export.schema.json`](../../schemas/backstage-export.schema.json) and [ADR 0053](../../docs/adr/0053-portal-export.md).

## 1. Produce the documents

In each repository, in CI after `gloss build`:

```sh
gloss export --backstage -o public/glossmark          # public/glossmark/<slug>.json + landscape.json
# or, once, across repositories:
gloss landscape --backstage public/glossmark ../repo-a ../repo-b
```

Links to code use `source-url` from `glossmark.yaml` (`https://github.com/o/r/blob/main/{path}#L{line}`) or `--source-url`. Serve the directory over HTTPS with CORS that lets the Backstage origin read it (static hosts do by default; a bucket needs `Access-Control-Allow-Origin`).

## 2. Install the plugin

The package is not published to npm yet. Copy or link `integrations/backstage` into your Backstage repository's `plugins/glossmark` (it is a standard frontend plugin package: `src/index.ts`, `createPlugin`, `backstage.role: frontend-plugin`) and add it to the app:

```sh
yarn --cwd packages/app add @glossmark/backstage-plugin-glossmark@link:../../plugins/glossmark
```

### Entity tab

In `packages/app/src/components/catalog/EntityPage.tsx`, on the Component and System pages:

```tsx
import { EntityGlossMarkContent, isGlossMarkAvailable } from '@glossmark/backstage-plugin-glossmark';

<EntityLayout.Route if={isGlossMarkAvailable} path="/glossmark" title="GlossMark">
  <EntityGlossMarkContent />
</EntityLayout.Route>
```

### Landscape page

In `packages/app/src/App.tsx`:

```tsx
import { GlossMarkLandscapePage } from '@glossmark/backstage-plugin-glossmark';

<Route path="/glossmark" element={<GlossMarkLandscapePage />} />
```

and a sidebar item pointing at `/glossmark` if you want one.

## 3. Point entities at their boundary

Either annotation on a Component or System names its document:

```yaml
# catalog-info.yaml
metadata:
  annotations:
    glossmark.dev/boundary: claimb:assessment        # CURIE or slug; the document is <glossmark.baseUrl>/claimb-assessment.json
    # or the full URL, when documents live in several places:
    glossmark.dev/export-url: https://docs.example.com/glossmark/claimb-assessment.json
```

A boundary CURIE slugs as `gloss` does: lower case, every run of characters outside `[a-z0-9]` one hyphen (`claimb:assessment` → `claimb-assessment`). The boundary is the `boundary:` of the `.gloss.yaml` manifest in the component's directory; the reverse join, deriving `owner` from `catalog-info.yaml`, is [issue #207](https://github.com/McKalvan/GlossMark/issues/207).

## 4. Configure the base URL

```yaml
# app-config.yaml
glossmark:
  baseUrl: https://docs.example.com/glossmark   # where landscape.json and the boundary documents are served
```

`glossmark.baseUrl` is required for the landscape page and for the `glossmark.dev/boundary` annotation; an entity with `glossmark.dev/export-url` needs nothing else.

## What the tab shows

| Card or table | From the document |
| --- | --- |
| Boundary | `boundary`: label, level, owner, criticality (and what it is inherited from), repository, path, part-of, deprecation |
| Coverage and drift | `coverage` (concepts with code and a test; files with an accepted link) and the count of open `findings` by kind, with the worst severity |
| Realizes, Holds, Presents, Hosts, Configures, Produces, Consumes, Publishes, Subscribes | `concepts`, one table per relation, highest criticality first: kind, criticality, status (`implemented`, `untested`, `unimplemented`, `orphan`, `planned`, `proposed`), whether the manifest declares it and the code derives it, the code under the boundary and the tests, linked through `source-url` |
| Open drift findings | `findings` that are not waived, worst first, each linked to the code or manifest line |
| Last review decisions | `decisions`: the newest accepted, rejected or stale links under the boundary, with the gate class (AUTO, SOFT, HARD) each had |

The landscape page lists `landscape.json`'s boundaries sorted by worst finding, then criticality, with repository, level, owner, concept count, a drift summary (`1 violation, 2 untested`), coverage and a link to each document; the context map (consumer → producer between boundaries) appears when `gloss landscape` wrote it.

## Developing

```sh
yarn install
yarn tsc     # type-check
yarn lint    # eslint
yarn test    # vitest: the data mapping against src/__fixtures__, which gloss export --backstage wrote for the claims-events showcase
yarn verify  # all three
```

Styling is Backstage's own (`@backstage/core-components` tables, cards and status icons, Material UI chips); there is no custom theme. The plugin targets the classic frontend system (`createPlugin`, `EntityLayout.Route`); a new-frontend-system (`createFrontendPlugin`) wrapper is a follow-up, as are the MkDocs and Docusaurus plugins that read the same documents.
