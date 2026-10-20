import { z } from 'zod';

/** "KF" + 10 digits + check digit, e.g. KF12345678903. */
export const TRACKING_NUMBER = /^KF\d{11}$/;

const isoInstant = z.string().datetime({ offset: true });

const addressSchema = z.object({
  line1: z.string().min(1),
  city: z.string().min(1),
  postcode: z.string().min(2),
  country: z.string().length(2),
  lat: z.number().min(-90).max(90),
  lng: z.number().min(-180).max(180),
});

const parcelSchema = z.object({
  parcelId: z.string().min(1),
  weightGrams: z.number().int().positive(),
  lengthCm: z.number().positive(),
  widthCm: z.number().positive(),
  heightCm: z.number().positive(),
});

const deliveryWindowSchema = z
  .object({ start: isoInstant, end: isoInstant })
  .refine((w) => Date.parse(w.start) < Date.parse(w.end), {
    message: 'deliveryWindow.start must precede deliveryWindow.end',
  });

/**
 * The ShipmentBooked payload as dispatch reads it. Unknown fields are stripped,
 * so shipment-core may add fields without breaking this consumer.
 */
// @gloss represents frt:ShipmentBooked conf=0.85
export const shipmentBookedSchema = z.object({
  shipmentId: z.string().min(1),
  trackingNumber: z.string().regex(TRACKING_NUMBER),
  shipperId: z.string().min(1),
  pickupAddress: addressSchema,
  deliveryAddress: addressSchema,
  parcels: z.array(parcelSchema).min(1),
  serviceLevel: z.enum(['STANDARD', 'EXPRESS']),
  carrierCode: z.string().min(1),
  deliveryWindow: deliveryWindowSchema,
  bookedAt: isoInstant,
});

export const shipmentBookedEnvelopeSchema = z.object({
  specversion: z.string().optional(),
  id: z.string().min(1),
  type: z.literal('ShipmentBooked'),
  source: z.string().optional(),
  time: isoInstant,
  data: shipmentBookedSchema,
});

export type ShipmentBookedData = z.infer<typeof shipmentBookedSchema>;
export type ShipmentBookedMessage = z.infer<typeof shipmentBookedEnvelopeSchema>;
