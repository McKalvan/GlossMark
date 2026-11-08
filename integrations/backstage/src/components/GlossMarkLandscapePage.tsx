import useAsync from 'react-use/lib/useAsync';
import Grid from '@material-ui/core/Grid';
import Typography from '@material-ui/core/Typography';
import {
  Content,
  ContentHeader,
  Header,
  InfoCard,
  Link,
  Page,
  Progress,
  ResponseErrorPanel,
  SupportButton,
  Table,
  TableColumn,
} from '@backstage/core-components';
import { useApi } from '@backstage/core-plugin-api';
import { glossMarkApiRef } from '../api';
import { coverageText, sortSummaries, summarizeFindings } from '../mapping';
import { BoundarySummary, LandscapeDocument } from '../types';
import { CriticalityBadge, SeverityStatus } from './Badges';

/**
 * Every boundary across repositories with its owner, criticality, concept
 * count, drift summary and coverage, from landscape.json; the context map
 * when gloss landscape wrote it.
 */
export function GlossMarkLandscapePage() {
  const api = useApi(glossMarkApiRef);
  const { value, loading, error } = useAsync(() => api.getLandscape(), [api]);
  return (
    <Page themeId="tool">
      <Header title="GlossMark landscape" subtitle="Boundaries, concepts and drift across repositories" />
      <Content>
        <ContentHeader title="">
          <SupportButton>Each row is a boundary: a manifest's directory in one repository. The document column is what the GlossMark tab of its entity shows.</SupportButton>
        </ContentHeader>
        {loading && <Progress />}
        {error && <ResponseErrorPanel error={error} />}
        {value && <LandscapeView land={value} baseUrl={api.baseUrl()} />}
      </Content>
    </Page>
  );
}

export function LandscapeView({ land, baseUrl }: { land: LandscapeDocument; baseUrl?: string }) {
  const rows = sortSummaries(land.boundaries);
  const columns: TableColumn<BoundarySummary>[] = [
    { title: '', width: '3%', sorting: false, render: r => <SeverityStatus severity={r.severity} /> },
    {
      title: 'Boundary',
      field: 'curie',
      render: r => (
        <span>
          <strong>{r.label ?? r.curie}</strong>
          <br />
          <code>{r.curie}</code>
          {r.deprecated ? ' (deprecated)' : ''}
        </span>
      ),
    },
    { title: 'Repository', field: 'repo', width: '12%' },
    { title: 'Level', field: 'level', width: '8%' },
    { title: 'Owner', field: 'owner', width: '12%' },
    { title: 'Criticality', field: 'criticality', width: '9%', render: r => <CriticalityBadge level={r.criticality} /> },
    { title: 'Concepts', field: 'concepts', width: '7%', type: 'numeric' },
    { title: 'Drift', sorting: false, render: r => summarizeFindings(r.findings) },
    { title: 'Coverage', width: '14%', render: r => coverageText(r), customSort: (a, b) => a.coverage.conceptCoveragePct - b.coverage.conceptCoveragePct },
    {
      title: 'Document',
      width: '8%',
      sorting: false,
      render: r => (baseUrl ? <Link to={`${baseUrl}/${r.document}`}>{r.document}</Link> : <code>{r.document}</code>),
    },
  ];
  const m = land.metrics;
  return (
    <Grid container spacing={3}>
      <Grid item xs={12} md={4}>
        <InfoCard title="Repositories">
          <Typography variant="body2" component="div">
            {land.repos.map(r => (
              <div key={r.name}>
                <strong>{r.name}</strong>: {r.boundaries} {r.boundaries === 1 ? 'boundary' : 'boundaries'}
              </div>
            ))}
            <div style={{ marginTop: 8, opacity: 0.7 }}>
              Generated {land.generated}
              {land.generator ? ` by ${land.generator}` : ''}
            </div>
          </Typography>
        </InfoCard>
      </Grid>
      <Grid item xs={12} md={8}>
        <InfoCard title="Totals">
          <Typography variant="body2" component="div">
            {m.concepts !== undefined && <div>{m.concepts} concepts, {m.boundaries} boundaries, {m.artifacts} artifacts</div>}
            {m.concepts_in_scope !== undefined && (
              <div>
                Concepts covered: {m.concepts_covered} of {m.concepts_in_scope} ({Math.round(m.concept_coverage_pct ?? 0)}%)
              </div>
            )}
            {m.violations !== undefined && (
              <div>
                Violations {m.violations} · gaps {m.gaps} · orphans {m.orphans} · untested {m.untested} · stale {m.stale} · proposed links {m.proposed}
              </div>
            )}
          </Typography>
        </InfoCard>
      </Grid>
      <Grid item xs={12}>
        <Table<BoundarySummary>
          title={`Boundaries (${rows.length})`}
          options={{ search: true, paging: rows.length > 50, pageSize: 50, padding: 'dense' }}
          columns={columns}
          data={rows}
        />
      </Grid>
      {land.contextMap.length > 0 && (
        <Grid item xs={12}>
          <InfoCard title="Context map (consumer → producer)">
            <Typography variant="body2" component="div">
              {land.contextMap.map((e, i) => (
                <div key={i}>
                  <code>{e.from}</code> → <code>{e.to}</code>
                  {e.via && e.via.length > 0 ? ` via ${e.via.join(', ')}` : ''}
                </div>
              ))}
            </Typography>
          </InfoCard>
        </Grid>
      )}
    </Grid>
  );
}
