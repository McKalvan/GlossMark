"use server";

import { redirect } from "next/navigation";
import { bookAppointment, cancelAppointment } from "@/lib/api";
import { requireSession } from "@/lib/session";

export async function book(formData: FormData) {
  const session = requireSession();
  const appt = await bookAppointment(
    session.token,
    session.patientId,
    String(formData.get("slotId")),
    String(formData.get("reason") ?? ""),
  );
  redirect(`/appointments/${appt.id}?booked=1`);
}

export async function cancel(formData: FormData) {
  const session = requireSession();
  await cancelAppointment(session.token, String(formData.get("appointmentId")));
  redirect("/appointments?cancelled=1");
}
