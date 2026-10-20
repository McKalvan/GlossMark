export type AppointmentStatus = "booked" | "cancelled" | "attended" | "no_show";

export interface Practitioner {
  id: string;
  displayName: string;
  specialty: string;
  clinicIds: string[];
  acceptingNewPatients: boolean;
}

export interface Slot {
  id: string;
  practitionerId: string;
  clinicId: string;
  startsAt: string;
  endsAt: string;
  booked: boolean;
}

export interface Appointment {
  id: string;
  patientId: string;
  practitionerId: string;
  slotId: string;
  startsAt: string;
  reason?: string;
  status: AppointmentStatus;
  cancelledAt?: string;
  lateCancel: boolean;
}
