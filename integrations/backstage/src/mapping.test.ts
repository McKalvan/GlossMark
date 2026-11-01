// @gloss verifies gmi:BackstagePlugin scope=file
import { describe, expect, it } from 'vitest';
import assessment from './__fixtures__/claimb-assessment.json';
import landscape from './__fixtures__/landscape.json';
import {
  agreement,
  checkBoundaryDocument,
  checkLandscapeDocument,
  countFindings,
  coverageText,
  fileCoverageText,
  groupConcepts,
  openFindings,
  slugOf,
  sortSummaries,
  sourceLink,
  summarizeFindings,
} from './mapping';

// The fixtures are what `gloss export --backstage` wrote for the
// claims-events showcase (claimb:assessment and its landscape.json).
const doc = checkBoundaryDocument(assessment);
const land = checkLandscapeDocument(landscape);

describe('boundary document', () => {
  it('is the assessment service with its governance', () => {
    expect(doc.boundary.curie).toBe('claimb:assessment');
    expect(doc.boundary.criticality).toBe('high');
    expect(doc.boundary.owner).toBe('claims-decisioning');
    expect(doc.boundary.partOf).toEqual(['claimb:claims-events']);
  });

  it('groups concepts by relation, highest criticality first', () => {
    const groups = groupConcepts(doc);
    const headings = groups.map(g => g.heading);
    expect(headings).toEqual(['Realizes', 'Configures', 'Produces', 'Consumes', 'Publishes', 'Subscribes']);
    const realizes = groups[0].concepts;
    expect(realizes[0].criticality).toBe('high');
    expect(realizes.map(c => c.curie)).toContain('claim:AutoApprovalLimit');
    // Within one level, by CURIE.
    const highs = realizes.filter(c => c.criticality === 'high').map(c => c.curie);
    expect(highs).toEqual([...highs].sort());
    for (const g of groups) {
      for (const c of g.concepts) expect(c.relations).toContain(g.relation);
    }
  });

  it('tells declared from derived', () => {
    const limit = doc.concepts.find(c => c.curie === 'claim:AutoApprovalLimit')!;
    expect(agreement(limit)).toBe('declared and derived');
    const referred = doc.concepts.find(c => c.curie === 'claim:ClaimReferred')!;
    expect(agreement(referred)).toBe('derived only');
    expect(referred.findings).toEqual(['violation:claimb:assessment:publishes:claim:ClaimReferred']);
  });

  it('keeps only the code under the boundary, with links', () => {
    for (const c of doc.concepts) {
      for (const a of c.artifacts) {
        expect(a.path.startsWith('services/assessment/')).toBe(true);
        expect(a.url).toBe(sourceLink(doc.sourceUrl, a.path, a.line));
      }
    }
    const limit = doc.concepts.find(c => c.curie === 'claim:AutoApprovalLimit')!;
    expect(limit.artifacts[0].via).toBe('tide:AutoApprovalRule');
    expect(limit.verifiers).toHaveLength(1);
  });

  it('sorts open findings worst first and counts them', () => {
    const open = openFindings(doc);
    expect(open[0].kind).toBe('violation');
    expect(open[0].url).toMatch(/referral\.py#L33$/);
    const counts = countFindings(doc.findings);
    expect(counts.worst).toBe('error');
    expect(counts.byKind.violation).toBe(1);
    expect(counts.total).toBe(doc.findings.filter(f => !f.waived).length);
  });

  it('words coverage', () => {
    expect(coverageText(doc)).toBe('3 of 4 concepts (75%)');
    expect(fileCoverageText(doc)).toBe('9 of 14 files (64%)');
    expect(coverageText({ coverage: { ...doc.coverage, conceptsInScope: 0 } })).toBe('no concepts in scope');
  });
});

describe('landscape', () => {
  it('lists every boundary with its document and summary', () => {
    expect(land.repos.map(r => r.name)).toEqual(['claims-events']);
    expect(land.boundaries).toHaveLength(8);
    const a = land.boundaries.find(b => b.curie === 'claimb:assessment')!;
    expect(a.document).toBe('claimb-assessment.json');
    expect(a.severity).toBe('error');
    expect(summarizeFindings(a.findings)).toBe('1 violation, 1 untested, 2 overlap');
  });

  it('sorts rows by severity, criticality, then CURIE', () => {
    const rows = sortSummaries(land.boundaries);
    const sev = rows.map(r => r.severity ?? 'none');
    const order = ['error', 'warning', 'info', 'none'];
    for (let i = 1; i < sev.length; i++) {
      expect(order.indexOf(sev[i])).toBeGreaterThanOrEqual(order.indexOf(sev[i - 1]));
    }
  });
});

describe('helpers', () => {
  it('slugs a CURIE as gloss does', () => {
    expect(slugOf('claimb:assessment')).toBe('claimb-assessment');
    expect(slugOf('bw:HireHQ_Invoicing')).toBe('bw-hirehq-invoicing');
    for (const b of land.boundaries) expect(slugOf(b.curie)).toBe(b.slug);
  });

  it('resolves source links like gloss', () => {
    const tpl = 'https://h.test/r/blob/main/{path}#L{line}';
    expect(sourceLink(tpl, 'a/b.py', 7)).toBe('https://h.test/r/blob/main/a/b.py#L7');
    expect(sourceLink(tpl, 'a/b.py')).toBe('https://h.test/r/blob/main/a/b.py');
    expect(sourceLink(undefined, 'a/b.py', 7)).toBe('');
    expect(sourceLink('vscode://file/x/{path}', 'a', 1)).toBe('');
  });

  it('refuses another format or kind', () => {
    expect(() => checkBoundaryDocument(landscape)).toThrow(/expected a boundary document/);
    expect(() => checkBoundaryDocument({ ...assessment, format: 2 })).toThrow(/format 2/);
    expect(() => checkLandscapeDocument(assessment)).toThrow(/expected landscape.json/);
  });
});
