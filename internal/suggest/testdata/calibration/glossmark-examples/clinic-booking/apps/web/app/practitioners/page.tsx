import Link from "next/link";
import { listPractitioners } from "@/lib/api";
import { requireSession } from "@/lib/session";

// @gloss implements .PractitionerDirectory conf=0.45
export default async function PractitionersPage({ searchParams }: { searchParams: { specialty?: string } }) {
  const session = requireSession();
  const practitioners = await listPractitioners(session.token, searchParams.specialty);
  return (
    <section>
      <h1>Our practitioners</h1>
      <ul>
        {practitioners.map((p) => (
          <li key={p.id} className="card">
            <strong>{p.displayName}</strong> · {p.specialty}
            {p.acceptingNewPatients ? (
              <Link href={`/book?practitioner=${p.id}`}> Book</Link>
            ) : (
              <span> · not accepting new patients</span>
            )}
          </li>
        ))}
      </ul>
    </section>
  );
}
