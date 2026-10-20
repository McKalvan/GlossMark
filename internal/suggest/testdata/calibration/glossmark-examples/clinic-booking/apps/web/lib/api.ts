import type { Appointment, Practitioner, Slot } from "./types";

const BASE = process.env.BOOKING_API_URL ?? "http://booking-api:8080";

async function call<T>(path: string, session: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}/v1${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${session}`, ...init?.headers },
    cache: "no-store",
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new ApiError(res.status, body.error ?? res.statusText);
  }
  return res.json() as Promise<T>;
}

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

export const listPractitioners = (session: string, specialty = "") =>
  call<Practitioner[]>(`/practitioners?specialty=${encodeURIComponent(specialty)}`, session);

export const listOpenSlots = (session: string, practitionerId: string, from: Date, to: Date) =>
  call<Slot[]>(`/practitioners/${practitionerId}/slots?from=${from.toISOString()}&to=${to.toISOString()}`, session);

export const bookAppointment = (session: string, patientId: string, slotId: string, reason: string) =>
  call<Appointment>("/appointments", session, { method: "POST", body: JSON.stringify({ patientId, slotId, reason }) });

export const cancelAppointment = (session: string, id: string) =>
  call<Appointment>(`/appointments/${id}/cancel`, session, { method: "POST" });
