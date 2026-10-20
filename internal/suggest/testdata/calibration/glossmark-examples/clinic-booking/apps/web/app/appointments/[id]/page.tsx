import { notFound } from "next/navigation";
import { format, differenceInHours, parseISO } from "date-fns";
import { requireSession } from "@/lib/session";
import { cancel } from "@/app/book/actions";
import type { Appointment } from "@/lib/types";

const CANCELLATION_WINDOW_HOURS = 24;

// @gloss implements .AppointmentPage conf=0.95
export default async function AppointmentPage({ params }: { params: { id: string } }) {
  const session = requireSession();
  const res = await fetch(`${process.env.READ_MODEL_URL}/appointments/${params.id}`, {
    headers: { Authorization: `Bearer ${session.token}` },
    cache: "no-store",
  });
  if (!res.ok) notFound();
  const appt = (await res.json()) as Appointment;
  const startsAt = parseISO(appt.startsAt);
  const late = differenceInHours(startsAt, new Date()) < CANCELLATION_WINDOW_HOURS;

  return (
    <section>
      <h1>Appointment on {format(startsAt, "EEEE d MMMM 'at' HH:mm")}</h1>
      <p>Status: {appt.status}</p>
      {appt.status === "booked" && (
        <form action={cancel}>
          <input type="hidden" name="appointmentId" value={appt.id} />
          {late && <p className="warning">This is less than 24 hours away and will count as a late cancellation.</p>}
          <button className="button" type="submit">Cancel appointment</button>
        </form>
      )}
    </section>
  );
}
