// The documents gloss export --backstage writes (format 1). Mirrors
// internal/backstage/backstage.go and schemas/backstage-export.schema.json
// in the GlossMark repository; keep the three in step.

export const FORMAT = 1;

export type Criticality = 'low' | 'medium' | 'high' | 'critical';

export type Relation =
  | 'realizes'
  | 'holds'
  | 'presents'
  | 'hosts'
  | 'configures'
  | 'produces'
  | 'consumes'
  | 'publishes'
  | 'subscribes';

export type ConceptStatus =
  | 'implemented'
  | 'untested'
  | 'unimplemented'
  | 'orphan'
  | 'planned'
  | 'proposed';

export type Severity = 'error' | 'warning' | 'info';

export interface Link {
  relation: string;
  path: string;
  line?: number;
  symbol?: string;
  url?: string;
  source?: 'inline' | 'manifest' | 'inferred';
  via?: string;
  proposed?: boolean;
}

export interface Concept {
  curie: string;
  iri: string;
  label?: string;
  kind?: string;
  tier?: 'core' | 'implementation';
  doc?: string;
  relations: Relation[];
  declared: boolean;
  derived: boolean;
  criticality?: Criticality;
  status?: ConceptStatus;
  deprecated?: boolean;
  replacedBy?: string;
  planned?: boolean;
  planBy?: string;
  artifacts: Link[];
  verifiers: Link[];
  findings?: string[];
}

export interface Coverage {
  conceptsInScope: number;
  conceptsCovered: number;
  conceptCoveragePct: number;
  inScopeFiles: number;
  coveredFiles: number;
  coveragePct: number;
}

export interface Finding {
  id: string;
  kind: string;
  severity: Severity;
  boundary?: string;
  relation?: string;
  concept?: string;
  declared?: string[];
  derived?: string[];
  msg: string;
  path?: string;
  line?: number;
  url?: string;
  by?: string;
  ticket?: string;
  waived?: boolean;
  waiver?: string;
}

export interface Decision {
  markerId: string;
  path: string;
  url?: string;
  relation: string;
  targets: string[];
  status: 'accepted' | 'rejected' | 'stale';
  actor?: string;
  created?: string;
  reviewedBy?: string;
  reviewedAt?: string;
  gate?: 'AUTO' | 'SOFT' | 'HARD';
  rationale?: string;
  reviewReason?: string;
}

export interface Boundary {
  curie: string;
  iri: string;
  slug: string;
  label?: string;
  level?: string;
  doc?: string;
  path: string;
  owner?: string;
  criticality?: Criticality;
  criticalityFrom?: string;
  partOf?: string[];
  deprecated?: boolean;
  replacedBy?: string;
  retireBy?: string;
}

/** One boundary document, `<slug>.json`. */
export interface BoundaryDocument {
  format: number;
  kind: 'boundary';
  generated: string;
  generator?: string;
  repo: string;
  sourceUrl?: string;
  boundary: Boundary;
  concepts: Concept[];
  coverage: Coverage;
  findings: Finding[];
  decisions: Decision[];
}

export interface RepoEntry {
  name: string;
  sourceUrl?: string;
  boundaries: number;
}

export interface BoundarySummary {
  curie: string;
  slug: string;
  document: string;
  repo: string;
  label?: string;
  level?: string;
  path: string;
  owner?: string;
  criticality?: Criticality;
  partOf?: string[];
  deprecated?: boolean;
  concepts: number;
  findings: Record<string, number>;
  severity?: Severity;
  coverage: Coverage;
}

export interface ContextEdge {
  from: string;
  to: string;
  via?: string[];
}

/** The index, `landscape.json`. */
export interface LandscapeDocument {
  format: number;
  kind: 'landscape';
  generated: string;
  generator?: string;
  repos: RepoEntry[];
  boundaries: BoundarySummary[];
  contextMap: ContextEdge[];
  metrics: Record<string, number>;
}
