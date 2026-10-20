import { requireSession } from "@/lib/session";

interface ClinicNoShow {
  clinicId: string;
  clinicName: string;
  rate: number;
  attended: number;
  noShows: number;
  lateCancellations: number;
}

// @gloss implements .NoShowDashboard conf=0.95
export default async function NoShowDashboard() {
  const session = requireSession();
  const res = await fetch(`${process.env.REPORTS_URL}/no-shows?weeks=4`, {
    headers: { Authorization: `Bearer ${session.token}` },
    next: { revalidate: 3600 },
  });
  const rows: ClinicNoShow[] = res.ok ? await res.json() : [];
  return (
    <section>
      <h1>No-show rate, last four weeks</h1>
      <table>
        <thead>
          <tr><th>Clinic</th><th>Rate</th><th>Attended</th><th>No-shows</th><th>Late cancellations</th></tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={r.clinicId}>
              <td>{r.clinicName}</td>
              <td>{(r.rate * 100).toFixed(1)}%</td>
              <td>{r.attended}</td>
              <td>{r.noShows}</td>
              <td>{r.lateCancellations}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}
