// Pure functions from the export documents to what the components show.
// Nothing here touches React or Backstage, so the tab's reading of a
// document is unit-tested against a fixture gloss export --backstage wrote.

import {
  BoundaryDocument,
  BoundarySummary,
  Concept,
  Criticality,
  Finding,
  FORMAT,
  LandscapeDocument,
  Relation,
  Severity,
} from './types';

/** Criticality levels, highest first: the sort order of every list. */
export const CRITICALITY_ORDER: Criticality[] = ['critical', 'high', 'medium', 'low'];

export function criticalityRank(c?: Criticality): number {
  const i = c ? CRITICALITY_ORDER.indexOf(c) : -1;
  return i < 0 ? CRITICALITY_ORDER.length : i;
}

/** The relation groups of the tab, in display order, with their headings. */
export const RELATION_GROUPS: { relation: Relation; heading: string }[] = [
  { relation: 'realizes', heading: 'Realizes' },
  { relation: 'holds', heading: 'Holds' },
  { relation: 'presents', heading: 'Presents' },
  { relation: 'hosts', heading: 'Hosts' },
  { relation: 'configures', heading: 'Configures' },
  { relation: 'produces', heading: 'Produces' },
  { relation: 'consumes', heading: 'Consumes' },
  { relation: 'publishes', heading: 'Publishes' },
  { relation: 'subscribes', heading: 'Subscribes' },
];

export interface ConceptGroup {
  relation: Relation;
  heading: string;
  concepts: Concept[];
}

/**
 * Groups a document's concepts by relation, every group sorted by
 * criticality (highest first) then CURIE; a concept under two relations is
 * in both groups. Empty groups are left out.
 */
export function groupConcepts(doc: BoundaryDocument): ConceptGroup[] {
  return RELATION_GROUPS.map(({ relation, heading }) => ({
    relation,
    heading,
    concepts: doc.concepts
      .filter(c => c.relations.includes(relation))
      .sort(compareConcepts),
  })).filter(g => g.concepts.length > 0);
}

export function compareConcepts(a: Concept, b: Concept): number {
  const r = criticalityRank(a.criticality) - criticalityRank(b.criticality);
  return r !== 0 ? r : a.curie.localeCompare(b.curie);
}

/** What the manifest says against what the code does, in one word. */
export function agreement(c: Concept): 'declared and derived' | 'declared only' | 'derived only' {
  if (c.declared && c.derived) return 'declared and derived';
  return c.declared ? 'declared only' : 'derived only';
}

/** Open findings: everything not waived, worst first, then by id. */
export function openFindings(doc: BoundaryDocument): Finding[] {
  return doc.findings.filter(f => !f.waived).sort(compareFindings);
}

export const SEVERITY_ORDER: Severity[] = ['error', 'warning', 'info'];

export function compareFindings(a: Finding, b: Finding): number {
  const r = SEVERITY_ORDER.indexOf(a.severity) - SEVERITY_ORDER.indexOf(b.severity);
  return r !== 0 ? r : a.id.localeCompare(b.id);
}

export interface FindingCounts {
  total: number;
  bySeverity: Record<Severity, number>;
  byKind: Record<string, number>;
  worst?: Severity;
}

export function countFindings(findings: Finding[]): FindingCounts {
  const out: FindingCounts = { total: 0, bySeverity: { error: 0, warning: 0, info: 0 }, byKind: {} };
  for (const f of findings) {
    if (f.waived) continue;
    out.total++;
    out.bySeverity[f.severity]++;
    out.byKind[f.kind] = (out.byKind[f.kind] ?? 0) + 1;
    if (!out.worst || SEVERITY_ORDER.indexOf(f.severity) < SEVERITY_ORDER.indexOf(out.worst)) {
      out.worst = f.severity;
    }
  }
  return out;
}

/** "3 of 4 concepts (75%)", or "no concepts in scope". */
export function coverageText(doc: Pick<BoundaryDocument, 'coverage'>): string {
  const c = doc.coverage;
  if (c.conceptsInScope === 0) return 'no concepts in scope';
  return `${c.conceptsCovered} of ${c.conceptsInScope} concepts (${Math.round(c.conceptCoveragePct)}%)`;
}

export function fileCoverageText(doc: Pick<BoundaryDocument, 'coverage'>): string {
  const c = doc.coverage;
  if (c.inScopeFiles === 0) return 'no files in scope';
  return `${c.coveredFiles} of ${c.inScopeFiles} files (${Math.round(c.coveragePct)}%)`;
}

/** A finding's one-line summary for the landscape: "1 violation, 2 untested". */
export function summarizeFindings(byKind: Record<string, number>): string {
  const kinds = Object.keys(byKind).sort((a, b) => kindRank(a) - kindRank(b) || a.localeCompare(b));
  if (kinds.length === 0) return 'clean';
  return kinds.map(k => `${byKind[k]} ${k}`).join(', ');
}

const KIND_ORDER = ['violation', 'value-drift', 'gap', 'rogue', 'dangling', 'stale', 'overdue', 'plan-overdue', 'orphan', 'untested', 'presented-only', 'unguarded', 'overlap', 'planned', 'proposed', 'deprecated-use'];

export function kindRank(kind: string): number {
  const i = KIND_ORDER.indexOf(kind);
  return i < 0 ? KIND_ORDER.length : i;
}

/** Landscape rows sorted by worst severity, then criticality, then CURIE. */
export function sortSummaries(rows: BoundarySummary[]): BoundarySummary[] {
  return [...rows].sort((a, b) => {
    const s = severityRank(a.severity) - severityRank(b.severity);
    if (s !== 0) return s;
    const c = criticalityRank(a.criticality) - criticalityRank(b.criticality);
    return c !== 0 ? c : a.curie.localeCompare(b.curie);
  });
}

export function severityRank(s?: Severity): number {
  const i = s ? SEVERITY_ORDER.indexOf(s) : -1;
  return i < 0 ? SEVERITY_ORDER.length : i;
}

/** The slug gloss gives a boundary CURIE (internal/backstage.Slug). */
export function slugOf(curie: string): string {
  return curie
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
}

/** Resolves a {path}/{line} template as gloss does; "" without one. */
export function sourceLink(template: string | undefined, path: string, line?: number): string {
  if (!template || !path || !/^https?:/.test(template)) return '';
  let u = template.replace('{path}', path);
  if (line && line > 0) return u.replace('{line}', String(line));
  const hash = u.indexOf('#');
  if (hash >= 0 && u.slice(hash).includes('{line}')) u = u.slice(0, hash);
  return u.replace('{line}', '');
}

/** Rejects a document of another format or kind before it is shown. */
export function checkBoundaryDocument(v: unknown): BoundaryDocument {
  const d = v as Partial<BoundaryDocument> | null;
  if (!d || typeof d !== 'object') throw new Error('not a GlossMark document');
  if (d.kind !== 'boundary') throw new Error(`expected a boundary document, got kind ${String(d.kind)}`);
  if (d.format !== FORMAT) throw new Error(`document format ${String(d.format)}; this plugin reads format ${FORMAT}: upgrade the plugin or gloss`);
  if (!d.boundary || !Array.isArray(d.concepts) || !Array.isArray(d.findings) || !d.coverage) {
    throw new Error('boundary document lacks boundary, concepts, findings or coverage');
  }
  return { ...d, decisions: d.decisions ?? [] } as BoundaryDocument;
}

export function checkLandscapeDocument(v: unknown): LandscapeDocument {
  const d = v as Partial<LandscapeDocument> | null;
  if (!d || typeof d !== 'object') throw new Error('not a GlossMark document');
  if (d.kind !== 'landscape') throw new Error(`expected landscape.json, got kind ${String(d.kind)}`);
  if (d.format !== FORMAT) throw new Error(`document format ${String(d.format)}; this plugin reads format ${FORMAT}: upgrade the plugin or gloss`);
  if (!Array.isArray(d.boundaries) || !Array.isArray(d.repos)) throw new Error('landscape.json lacks boundaries or repos');
  return { ...d, contextMap: d.contextMap ?? [], metrics: d.metrics ?? {} } as LandscapeDocument;
}
