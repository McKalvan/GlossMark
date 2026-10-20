// @gloss verifies shc:ShipmentBookedEmitter, .BookedIntake scope=file conf=0.85 note="consumer-driven contract for the payload shipment-core publishes"
/**
 * Consumer-driven contract for shipment.booked.v1.
 *
 * The message below is the shape shipment-core's outbox relay publishes. This test
 * pins the fields dispatch depends on; if shipment-core renames or retypes any of
 * them, this test fails before the change reaches the shared topic.
 */
import { describe, expect, it } from 'vitest';
import type { PlanningSettings } from '../../src/config.js';
import { pickupDateFor, toPickupRequest } from '../../src/consumer/shipmentBookedConsumer.js';
import { shipmentBookedEnvelopeSchema } from '../../src/consumer/schemas.js';

const publishedByShipmentCore = {
  specversion: '1.0',
  id: '0b8f6c2e-7d0a-4b8e-9f3c-5a1d2e3f4b6c',
  type: 'ShipmentBooked',
  source: 'urn:kestrel:shipment-core',
  time: '2026-10-19T08:41:07.512Z',
  datacontenttype: 'application/json',
  data: {
    shipmentId: 'shp_01JA4K7XQ3M9T2V8',
    trackingNumber: 'KF12345678903',
    shipperId: 'shpr_00841',
    pickupAddress: {
      line1: '14 Clerkenwell Close',
      city: 'London',
      postcode: 'EC1R 0AA',
      country: 'GB',
      lat: 51.5236,
      lng: -0.1059,
    },
    deliveryAddress: {
      line1: '3 Albion Street',
      city: 'Leeds',
      postcode: 'LS1 5ES',
      country: 'GB',
      lat: 53.7983,
      lng: -1.5466,
    },
    parcels: [
      { parcelId: 'prc_1', weightGrams: 2350, lengthCm: 40, widthCm: 30, heightCm: 20 },
      { parcelId: 'prc_2', weightGrams: 800, lengthCm: 25, widthCm: 20, heightCm: 10 },
    ],
    serviceLevel: 'EXPRESS',
    carrierCode: 'KFX',
    deliveryWindow: { start: '2026-10-20T09:00:00.000Z', end: '2026-10-20T11:00:00.000Z' },
    bookedAt: '2026-10-19T08:41:05.000Z',
    consignmentId: null,
  },
};

const settings: PlanningSettings = {
  depot: { lat: 51.5265, lng: -0.2667 },
  timeZone: 'Europe/London',
  expressCutoff: '11:00',
  avgSpeedKmh: 28,
  serviceMinutes: 6,
  etaWindowMinutes: 30,
  linehaulBufferMinutes: 240,
  minLeadMinutes: 20,
};

const clone = (): typeof publishedByShipmentCore => structuredClone(publishedByShipmentCore);

describe('contract: shipment.booked.v1 (shipment-core -> dispatch)', () => {
  it('accepts the message shipment-core publishes, ignoring fields dispatch does not use', () => {
    const parsed = shipmentBookedEnvelopeSchema.safeParse(publishedByShipmentCore);

    expect(parsed.success).toBe(true);
  });

  it('exposes every field dispatch relies on', () => {
    const { data } = shipmentBookedEnvelopeSchema.parse(publishedByShipmentCore);

    expect(data.shipmentId).toBe('shp_01JA4K7XQ3M9T2V8');
    expect(data.trackingNumber).toMatch(/^KF\d{11}$/);
    expect(data.pickupAddress).toEqual(publishedByShipmentCore.data.pickupAddress);
    expect(data.parcels.map((p) => p.weightGrams)).toEqual([2350, 800]);
    expect(data.serviceLevel).toBe('EXPRESS');
    expect(data.deliveryWindow).toEqual(publishedByShipmentCore.data.deliveryWindow);
    expect(data.bookedAt).toBe('2026-10-19T08:41:05.000Z');
  });

  it('maps onto a queued pickup request', () => {
    const { data } = shipmentBookedEnvelopeSchema.parse(publishedByShipmentCore);
    const request = toPickupRequest(data, settings);

    expect(request).toMatchObject({
      shipmentId: 'shp_01JA4K7XQ3M9T2V8',
      status: 'QUEUED',
      zone: 'EC',
      parcelCount: 2,
      totalWeightGrams: 3150,
      lat: 51.5236,
      lng: -0.1059,
    });
    // Booked 09:41 BST, before the 11:00 cut-off: an express pickup the same day.
    expect(pickupDateFor(data, settings)).toBe('2026-10-19');
  });

  it.each([
    ['pickupAddress.lat', (m: ReturnType<typeof clone>) => delete (m.data.pickupAddress as Partial<typeof m.data.pickupAddress>).lat],
    ['parcels', (m: ReturnType<typeof clone>) => (m.data.parcels = [])],
    ['serviceLevel', (m: ReturnType<typeof clone>) => ((m.data as { serviceLevel: string }).serviceLevel = 'PRIORITY')],
    ['trackingNumber', (m: ReturnType<typeof clone>) => (m.data.trackingNumber = '1Z999AA10123456784')],
    ['deliveryWindow', (m: ReturnType<typeof clone>) => (m.data.deliveryWindow = { start: m.data.deliveryWindow.end, end: m.data.deliveryWindow.start })],
  ])('rejects a message with a broken %s', (_field, mutate) => {
    const message = clone();
    mutate(message);

    expect(shipmentBookedEnvelopeSchema.safeParse(message).success).toBe(false);
  });
});
