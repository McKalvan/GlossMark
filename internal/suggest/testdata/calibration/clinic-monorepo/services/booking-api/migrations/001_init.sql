-- @gloss represents clinic:Slot
CREATE TABLE slots (id TEXT PRIMARY KEY, practitioner TEXT, starts TIMESTAMP);

CREATE TYPE appointment_status AS ENUM ('booked', 'cancelled', 'noshow');

-- @gloss represents clinic:Appointment
CREATE TABLE appointments (id TEXT PRIMARY KEY, slot TEXT, patient TEXT, status appointment_status);

CREATE INDEX appointments_slot_idx ON appointments (slot);

-- @gloss represents clinic:Practitioner
CREATE TABLE practitioners (id TEXT PRIMARY KEY, name TEXT);
