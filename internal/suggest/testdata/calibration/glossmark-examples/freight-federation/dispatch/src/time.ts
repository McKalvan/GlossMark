export const ISO_DATE = /^\d{4}-\d{2}-\d{2}$/;

const MS_PER_MINUTE = 60_000;

interface LocalParts {
  year: number;
  month: number;
  day: number;
  hour: number;
  minute: number;
  second: number;
}

const formatters = new Map<string, Intl.DateTimeFormat>();

function formatterFor(timeZone: string): Intl.DateTimeFormat {
  let formatter = formatters.get(timeZone);
  if (!formatter) {
    formatter = new Intl.DateTimeFormat('en-GB', {
      timeZone,
      hourCycle: 'h23',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    });
    formatters.set(timeZone, formatter);
  }
  return formatter;
}

export function localParts(at: Date, timeZone: string): LocalParts {
  const values: Record<string, number> = {};
  for (const part of formatterFor(timeZone).formatToParts(at)) {
    if (part.type !== 'literal') values[part.type] = Number(part.value);
  }
  return {
    year: values.year,
    month: values.month,
    day: values.day,
    hour: values.hour,
    minute: values.minute,
    second: values.second,
  };
}

const pad = (n: number): string => String(n).padStart(2, '0');

export function localDate(at: Date, timeZone: string): string {
  const p = localParts(at, timeZone);
  return `${p.year}-${pad(p.month)}-${pad(p.day)}`;
}

export function localMinutesOfDay(at: Date, timeZone: string): number {
  const p = localParts(at, timeZone);
  return p.hour * 60 + p.minute;
}

export function parseClock(hhmm: string): number {
  const [hours, minutes] = hhmm.split(':').map(Number);
  return hours * 60 + minutes;
}

function offsetMinutes(at: Date, timeZone: string): number {
  const p = localParts(at, timeZone);
  const wallClockAsUtc = Date.UTC(p.year, p.month - 1, p.day, p.hour, p.minute, p.second);
  const wholeSeconds = Math.floor(at.getTime() / 1000) * 1000;
  return Math.round((wallClockAsUtc - wholeSeconds) / MS_PER_MINUTE);
}

/** The instant at which the wall clock in `timeZone` shows `hhmm` on `date` (YYYY-MM-DD). */
export function zonedDateTime(date: string, hhmm: string, timeZone: string): Date {
  const [year, month, day] = date.split('-').map(Number);
  const naive = Date.UTC(year, month - 1, day, 0, parseClock(hhmm));
  const firstGuess = naive - offsetMinutes(new Date(naive), timeZone) * MS_PER_MINUTE;
  // Re-read the offset at the guessed instant so dates on a DST change resolve correctly.
  return new Date(naive - offsetMinutes(new Date(firstGuess), timeZone) * MS_PER_MINUTE);
}

export function addDays(date: string, days: number): string {
  const [year, month, day] = date.split('-').map(Number);
  return new Date(Date.UTC(year, month - 1, day + days)).toISOString().slice(0, 10);
}

export function addMinutes(at: Date, minutes: number): Date {
  return new Date(at.getTime() + minutes * MS_PER_MINUTE);
}

/** Postgres DATE columns are exchanged with Prisma as UTC midnight. */
export function toDbDate(date: string): Date {
  return new Date(`${date}T00:00:00.000Z`);
}

export function fromDbDate(value: Date): string {
  return value.toISOString().slice(0, 10);
}
