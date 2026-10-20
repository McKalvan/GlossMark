# Architecture

Larkspur Clinics runs outpatient clinics. Patients book, move and cancel appointments online; clinic staff see the schedule and the no-show report.

<!-- @gloss documents clinic:Booking conf=0.9 -->
## Booking

The web app (`apps/web`) is a Next.js server-rendered site. It calls the booking API (`services/booking-api`, Go) with the patient's session. The API owns the rules: a slot can be booked once, a patient cannot hold two overlapping appointments, and a cancellation inside the cancellation window is flagged as late.

<!-- @gloss documents clinic:AppointmentReminding conf=0.9 -->
## Reminders

Every booking and cancellation is written to a transactional outbox in Postgres and relayed to an SQS FIFO queue. The reminder worker (`services/reminders`, Python) plans an SMS or email reminder 48 hours ahead and a nudge on the day, and withdraws them when the appointment is cancelled.

<!-- @gloss documents clinic:NoShowRate conf=0.9 -->
## No-show rate

A weekly CronJob computes the no-show rate per clinic: no-shows plus late cancellations over every appointment that should have happened. Operations reviews it on Mondays.

## Data

One Postgres database holds clinics, practitioners, slots, patients, appointments, the outbox and scheduled reminders. Patient rows are PII; the database is encrypted with its own KMS key and backed up for 35 days.

## Infrastructure

Terraform in `infra/terraform` provisions the database, the queue and the EKS cluster. Kubernetes manifests in `deploy/k8s` run the three services. GitHub Actions builds images and rolls them out.
