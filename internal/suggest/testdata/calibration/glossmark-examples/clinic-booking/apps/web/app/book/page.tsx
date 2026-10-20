import { addDays } from "date-fns";
import { listOpenSlots, listPractitioners } from "@/lib/api";
import { requireSession } from "@/lib/session";
import { SlotPicker } from "@/components/SlotPicker";

// @gloss implements .BookingPage conf=0.91
export default async function BookPage({ searchParams }: { searchParams: { practitioner?: string } }) {
  const session = requireSession();
  const practitioners = await listPractitioners(session.token);
  const selected = searchParams.practitioner ?? practitioners[0]?.id;
  const now = new Date();
  const slots = selected ? await listOpenSlots(session.token, selected, now, addDays(now, 14)) : [];

  return (
    <section>
      <h1>Find a time</h1>
      <form method="get">
        <select name="practitioner" defaultValue={selected}>
          {practitioners.map((p) => (
            <option key={p.id} value={p.id}>
              {p.displayName} · {p.specialty}
            </option>
          ))}
        </select>
        <button className="button" type="submit">Show times</button>
      </form>
      <SlotPicker slots={slots} patientId={session.patientId} />
    </section>
  );
}
