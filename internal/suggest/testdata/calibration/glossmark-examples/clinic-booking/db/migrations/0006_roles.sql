CREATE ROLE booking_api NOLOGIN;
CREATE ROLE reminders NOLOGIN;

GRANT SELECT, INSERT, UPDATE ON appointments, slots, outbox TO booking_api;
GRANT SELECT ON clinics, practitioners, practitioner_clinics, patients TO booking_api;
GRANT SELECT ON appointments, slots, patients TO reminders;
GRANT SELECT, INSERT, UPDATE, DELETE ON reminders TO reminders;
