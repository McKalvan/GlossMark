import { requireSession } from "@/lib/session";
import { AppointmentCard } from "@/components/AppointmentCard";
import type { Appointment } from "@/lib/types";

async function upcoming(token: string): Promise<Appointment[]> {
  const res = await fetch(`${process.env.READ_MODEL_URL}/patients/me/appointments`, {
    headers: { Authorization: `Bearer ${token}` },
    cache: "no-store",
  });
  return res.ok ? res.json() : [];
}

// @gloss implements .AppointmentPage conf=0.91
export default async function AppointmentsPage({ searchParams }: { searchParams: { cancelled?: string } }) {
  const session = requireSession();
  const appointments = await upcoming(session.token);
  return (
    <section>
      <h1>Your appointments</h1>
      {searchParams.cancelled && <p role="status">Your appointment was cancelled.</p>}
      {appointments.length === 0 && <p>You have no upcoming appointments.</p>}
      {appointments.map((a) => (
        <AppointmentCard key={a.id} appointment={a} />
      ))}
    </section>
  );
}
