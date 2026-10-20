import Link from "next/link";
import { format, parseISO } from "date-fns";
import type { Appointment } from "@/lib/types";

// @gloss presents clinic:Appointment conf=0.8
export function AppointmentCard({ appointment }: { appointment: Appointment }) {
  return (
    <article className="card">
      <h2>{format(parseISO(appointment.startsAt), "EEEE d MMMM, HH:mm")}</h2>
      <p>Status: {appointment.status}</p>
      <Link href={`/appointments/${appointment.id}`}>Details</Link>
    </article>
  );
}
