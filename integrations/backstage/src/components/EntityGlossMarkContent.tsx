import React from 'react';
import useAsync from 'react-use/lib/useAsync';
import Grid from '@material-ui/core/Grid';
import Typography from '@material-ui/core/Typography';
import Chip from '@material-ui/core/Chip';
import {
  EmptyState,
  InfoCard,
  Link,
  MissingAnnotationEmptyState,
  Progress,
  ResponseErrorPanel,
  Table,
  TableColumn,
} from '@backstage/core-components';
import { useApi } from '@backstage/core-plugin-api';
import { useEntity } from '@backstage/plugin-catalog-react';
import { boundaryOf, EXPORT_URL_ANNOTATION, exportUrlOf } from '../annotations';
import { glossMarkApiRef } from '../api';
import {
  agreement,
  countFindings,
  coverageText,
  fileCoverageText,
  groupConcepts,
  openFindings,
} from '../mapping';
import { BoundaryDocument, Concept, Decision, Finding, Link as CodeLink } from '../types';
import { CriticalityBadge, GateBadge, SeverityStatus } from './Badges';

/**
 * The GlossMark tab of a Component or System: the boundary's concepts by
 * relation with criticality, its coverage, its open drift findings linked
 * to code and the last review decisions under it, read from the document
 * named by the glossmark.dev/export-url annotation, or built from
 * glossmark.baseUrl and the glossmark.dev/boundary annotation.
 */
export function EntityGlossMarkContent() {
  const { entity } = useEntity();
  const api = useApi(glossMarkApiRef);
  const url = exportUrlOf(entity) ?? (boundaryOf(entity) ? api.documentUrl(boundaryOf(entity)!) : undefined);
  const { value, loading, error } = useAsync(async () => (url ? api.getBoundary(url) : undefined), [url]);

  if (!exportUrlOf(entity) && !boundaryOf(entity)) {
    return <MissingAnnotationEmptyState annotation={EXPORT_URL_ANNOTATION} />;
  }
  if (!url) {
    return (
      <EmptyState
        missing="info"
        title="GlossMark base URL not configured"
        description="The entity names its boundary with glossmark.dev/boundary, but glossmark.baseUrl is not set in app-config.yaml. Point it at the directory gloss export --backstage wrote, or give the entity a glossmark.dev/export-url annotation."
      />
    );
  }
  if (loading) return <Progress />;
  if (error) return <ResponseErrorPanel error={error} />;
  if (!value) return null;
  return <BoundaryView doc={value} />;
}

export function BoundaryView({ doc }: { doc: BoundaryDocument }) {
  const b = doc.boundary;
  const findings = openFindings(doc);
  const counts = countFindings(doc.findings);
  const groups = groupConcepts(doc);
  return (
    <Grid container spacing={3}>
      <Grid item xs={12} md={6}>
        <InfoCard title={b.label ?? b.curie} subheader={b.curie}>
          <Typography variant="body2" component="div">
            <dl>
              {b.level && <Row k="Level" v={b.level} />}
              {b.owner && <Row k="Owner" v={b.owner} />}
              <Row k="Criticality" v={<CriticalityBadge level={b.criticality} from={b.criticalityFrom} />} />
              <Row k="Repository" v={doc.repo} />
              <Row k="Path" v={<code>{b.path || '/'}</code>} />
              {b.partOf && b.partOf.length > 0 && <Row k="Part of" v={b.partOf.join(', ')} />}
              {b.deprecated && <Row k="Deprecated" v={`replaced by ${b.replacedBy ?? '—'}${b.retireBy ? `, retire by ${b.retireBy}` : ''}`} />}
              {b.doc && <Row k="About" v={b.doc} />}
              <Row k="Generated" v={`${doc.generated}${doc.generator ? ` by ${doc.generator}` : ''}`} />
            </dl>
          </Typography>
        </InfoCard>
      </Grid>
      <Grid item xs={12} md={6}>
        <InfoCard title="Coverage and drift">
          <Typography variant="body2" component="div">
            <dl>
              <Row k="Concepts covered" v={coverageText(doc)} />
              <Row k="Files with an accepted link" v={fileCoverageText(doc)} />
              <Row
                k="Open findings"
                v={
                  <SeverityStatus severity={counts.worst}>
                    {counts.total === 0
                      ? 'none'
                      : Object.entries(counts.byKind)
                          .map(([k, n]) => `${n} ${k}`)
                          .join(', ')}
                  </SeverityStatus>
                }
              />
            </dl>
          </Typography>
        </InfoCard>
      </Grid>
      {groups.map(g => (
        <Grid item xs={12} key={g.relation}>
          <ConceptTable title={g.heading} concepts={g.concepts} />
        </Grid>
      ))}
      {groups.length === 0 && (
        <Grid item xs={12}>
          <EmptyState missing="content" title="No concepts" description="The manifest declares nothing and no code under the boundary links to a concept." />
        </Grid>
      )}
      <Grid item xs={12}>
        <FindingTable findings={findings} />
      </Grid>
      {doc.decisions.length > 0 && (
        <Grid item xs={12}>
          <DecisionTable decisions={doc.decisions} />
        </Grid>
      )}
    </Grid>
  );
}

function Row({ k, v }: { k: string; v: React.ReactNode }) {
  return (
    <div style={{ display: 'flex', gap: 8, marginBottom: 4 }}>
      <dt style={{ minWidth: 180, opacity: 0.7 }}>{k}</dt>
      <dd style={{ margin: 0 }}>{v}</dd>
    </div>
  );
}

function CodeLinks({ links }: { links: CodeLink[] }) {
  if (links.length === 0) return <span style={{ opacity: 0.6 }}>—</span>;
  return (
    <>
      {links.map((l, i) => {
        const text = `${l.path}${l.line ? `:${l.line}` : ''}${l.symbol ? ` ${l.symbol}` : ''}`;
        const title = [l.relation, l.via ? `via ${l.via}` : '', l.source ? `(${l.source})` : '', l.proposed ? 'proposed' : ''].filter(Boolean).join(' ');
        return (
          <div key={`${l.path}:${l.line}:${i}`} title={title}>
            {l.url ? <Link to={l.url}>{text}</Link> : <code>{text}</code>}
          </div>
        );
      })}
    </>
  );
}

function ConceptTable({ title, concepts }: { title: string; concepts: Concept[] }) {
  const columns: TableColumn<Concept>[] = [
    {
      title: 'Concept',
      field: 'curie',
      render: c => (
        <span title={c.doc}>
          <strong>{c.label ?? c.curie.split(':').pop()}</strong>
          <br />
          <code>{c.curie}</code>
          {c.deprecated && <Chip size="small" variant="outlined" label={`deprecated${c.replacedBy ? ` → ${c.replacedBy}` : ''}`} style={{ marginLeft: 4 }} />}
          {c.planned && <Chip size="small" variant="outlined" label={`planned${c.planBy ? ` by ${c.planBy}` : ''}`} style={{ marginLeft: 4 }} />}
        </span>
      ),
    },
    { title: 'Kind', field: 'kind', width: '8%' },
    { title: 'Criticality', field: 'criticality', width: '10%', render: c => <CriticalityBadge level={c.criticality} /> },
    { title: 'Status', field: 'status', width: '10%', render: c => c.status ?? <span style={{ opacity: 0.6 }}>imported</span> },
    { title: 'Agreement', width: '12%', render: c => agreement(c), customSort: (a, b) => agreement(a).localeCompare(agreement(b)) },
    { title: 'Code', sorting: false, render: c => <CodeLinks links={c.artifacts} /> },
    { title: 'Tests', sorting: false, render: c => <CodeLinks links={c.verifiers} /> },
  ];
  return (
    <Table<Concept>
      title={`${title} (${concepts.length})`}
      options={{ search: false, paging: concepts.length > 20, pageSize: 20, padding: 'dense' }}
      columns={columns}
      data={concepts}
    />
  );
}

function FindingTable({ findings }: { findings: Finding[] }) {
  const columns: TableColumn<Finding>[] = [
    { title: '', width: '4%', sorting: false, render: f => <SeverityStatus severity={f.severity} /> },
    { title: 'Kind', field: 'kind', width: '10%' },
    { title: 'Concept', field: 'concept', width: '18%', render: f => <code>{f.concept ?? ''}</code> },
    { title: 'What', field: 'msg' },
    {
      title: 'Where',
      field: 'path',
      width: '22%',
      render: f => {
        if (!f.path) return null;
        const text = `${f.path}${f.line ? `:${f.line}` : ''}`;
        return f.url ? <Link to={f.url}>{text}</Link> : <code>{text}</code>;
      },
    },
  ];
  return (
    <Table<Finding>
      title={`Open drift findings (${findings.length})`}
      options={{ search: false, paging: findings.length > 20, pageSize: 20, padding: 'dense' }}
      columns={columns}
      data={findings}
      emptyContent={<div style={{ padding: 16 }}>No open findings: the manifest and the code agree.</div>}
    />
  );
}

function DecisionTable({ decisions }: { decisions: Decision[] }) {
  const columns: TableColumn<Decision>[] = [
    { title: 'When', field: 'reviewedAt', width: '16%', defaultSort: 'desc' },
    { title: 'Decision', field: 'status', width: '10%' },
    { title: 'Gate', field: 'gate', width: '8%', render: d => <GateBadge gate={d.gate} /> },
    { title: 'By', field: 'reviewedBy', width: '14%' },
    { title: 'Link', sorting: false, render: d => <span><code>{d.relation}</code> {d.targets.join(', ')}</span> },
    { title: 'Where', field: 'path', render: d => (d.url ? <Link to={d.url}>{d.path}</Link> : <code>{d.path}</code>) },
    { title: 'Why', sorting: false, render: d => d.reviewReason ?? d.rationale ?? '' },
  ];
  return (
    <Table<Decision>
      title={`Last review decisions (${decisions.length})`}
      options={{ search: false, paging: false, padding: 'dense' }}
      columns={columns}
      data={decisions}
    />
  );
}
