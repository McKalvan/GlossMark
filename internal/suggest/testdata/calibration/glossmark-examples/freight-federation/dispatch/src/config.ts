import { z } from 'zod';
import type { LatLng } from './planning/geo.js';

const clockTime = z.string().regex(/^([01]\d|2[0-3]):[0-5]\d$/, 'expected HH:MM');

function isTimeZone(value: string): boolean {
  try {
    new Intl.DateTimeFormat('en-GB', { timeZone: value });
    return true;
  } catch {
    return false;
  }
}

const envSchema = z.object({
  NODE_ENV: z.enum(['development', 'test', 'production']).default('development'),
  PORT: z.coerce.number().int().positive().default(8080),
  LOG_LEVEL: z.enum(['fatal', 'error', 'warn', 'info', 'debug', 'trace']).default('info'),
  DATABASE_URL: z.string().url(),
  KAFKA_BROKERS: z
    .string()
    .min(1)
    .transform((value) =>
      value
        .split(',')
        .map((broker) => broker.trim())
        .filter((broker) => broker.length > 0),
    ),
  KAFKA_CLIENT_ID: z.string().min(1).default('dispatch'),
  KAFKA_GROUP_ID: z.string().min(1).default('dispatch.shipment-booked'),
  DEPOT_LAT: z.coerce.number().min(-90).max(90),
  DEPOT_LNG: z.coerce.number().min(-180).max(180),
  DEPOT_TIMEZONE: z.string().refine(isTimeZone, 'unknown IANA time zone').default('Europe/London'),
  EXPRESS_CUTOFF: clockTime.default('11:00'),
  AVG_SPEED_KMH: z.coerce.number().positive().default(28),
  SERVICE_MINUTES: z.coerce.number().nonnegative().default(6),
  ETA_WINDOW_MINUTES: z.coerce.number().int().positive().default(30),
  LINEHAUL_BUFFER_MINUTES: z.coerce.number().int().nonnegative().default(240),
  MIN_LEAD_MINUTES: z.coerce.number().int().nonnegative().default(20),
});

export type Config = z.infer<typeof envSchema>;

export interface PlanningSettings {
  depot: LatLng;
  timeZone: string;
  expressCutoff: string;
  avgSpeedKmh: number;
  serviceMinutes: number;
  etaWindowMinutes: number;
  linehaulBufferMinutes: number;
  minLeadMinutes: number;
}

export function loadConfig(env: NodeJS.ProcessEnv = process.env): Config {
  const parsed = envSchema.safeParse(env);
  if (!parsed.success) {
    const details = parsed.error.issues
      .map((issue) => `  ${issue.path.join('.')}: ${issue.message}`)
      .join('\n');
    throw new Error(`Invalid configuration:\n${details}`);
  }
  return parsed.data;
}

export function planningSettings(config: Config): PlanningSettings {
  return {
    depot: { lat: config.DEPOT_LAT, lng: config.DEPOT_LNG },
    timeZone: config.DEPOT_TIMEZONE,
    expressCutoff: config.EXPRESS_CUTOFF,
    avgSpeedKmh: config.AVG_SPEED_KMH,
    serviceMinutes: config.SERVICE_MINUTES,
    etaWindowMinutes: config.ETA_WINDOW_MINUTES,
    linehaulBufferMinutes: config.LINEHAUL_BUFFER_MINUTES,
    minLeadMinutes: config.MIN_LEAD_MINUTES,
  };
}
