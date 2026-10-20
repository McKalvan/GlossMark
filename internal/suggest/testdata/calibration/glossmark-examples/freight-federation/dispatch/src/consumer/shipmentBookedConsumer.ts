import type { Prisma, PrismaClient } from '@prisma/client';
import type { Consumer, Kafka, KafkaMessage } from 'kafkajs';
import type { BaseLogger } from 'pino';
import type { PlanningSettings } from '../config.js';
import { TOPICS } from '../events/types.js';
import { isWithinExpressCutoff } from '../planning/expressPickup.js';
import { zoneForPostcode } from '../planning/geo.js';
import { addDays, localDate, toDbDate } from '../time.js';
import { shipmentBookedEnvelopeSchema, type ShipmentBookedData } from './schemas.js';

/**
 * Standard pickups are collected the next day. Express bookings made before the
 * cut-off are queued for the same day; later ones fall back to the next day.
 */
export function pickupDateFor(event: ShipmentBookedData, settings: PlanningSettings): string {
  const bookedAt = new Date(event.bookedAt);
  const bookedDay = localDate(bookedAt, settings.timeZone);
  if (
    event.serviceLevel === 'EXPRESS' &&
    isWithinExpressCutoff(bookedAt, bookedDay, settings.timeZone, settings.expressCutoff)
  ) {
    return bookedDay;
  }
  return addDays(bookedDay, 1);
}

export function toPickupRequest(
  event: ShipmentBookedData,
  settings: PlanningSettings,
): Prisma.PickupRequestCreateInput {
  const { pickupAddress } = event;
  return {
    shipmentId: event.shipmentId,
    trackingNumber: event.trackingNumber,
    shipperId: event.shipperId,
    serviceLevel: event.serviceLevel,
    status: 'QUEUED',
    pickupDate: toDbDate(pickupDateFor(event, settings)),
    zone: zoneForPostcode(pickupAddress.postcode),
    line1: pickupAddress.line1,
    city: pickupAddress.city,
    postcode: pickupAddress.postcode,
    country: pickupAddress.country,
    lat: pickupAddress.lat,
    lng: pickupAddress.lng,
    parcelCount: event.parcels.length,
    totalWeightGrams: event.parcels.reduce((sum, parcel) => sum + parcel.weightGrams, 0),
    deliveryWindowStart: new Date(event.deliveryWindow.start),
    deliveryWindowEnd: new Date(event.deliveryWindow.end),
    bookedAt: new Date(event.bookedAt),
  };
}

// @gloss implements .BookedIntake conf=0.9
export class ShipmentBookedConsumer {
  private readonly consumer: Consumer;

  constructor(
    kafka: Kafka,
    groupId: string,
    private readonly prisma: PrismaClient,
    private readonly settings: PlanningSettings,
    private readonly logger: BaseLogger,
  ) {
    this.consumer = kafka.consumer({ groupId, sessionTimeout: 30_000 });
  }

  // @gloss subscribes frt:ShipmentBooked conf=0.9
  async start(): Promise<void> {
    await this.consumer.connect();
    await this.consumer.subscribe({ topic: TOPICS.shipmentBooked, fromBeginning: false });
    await this.consumer.run({
      eachMessage: async ({ partition, message }) => this.handle(message, partition),
    });
  }

  async stop(): Promise<void> {
    await this.consumer.disconnect();
  }

  /**
   * Malformed messages are logged and skipped so one bad event cannot block the
   * partition. Database errors propagate and kafkajs retries the message.
   */
  async handle(message: KafkaMessage, partition: number): Promise<void> {
    const position = { topic: TOPICS.shipmentBooked, partition, offset: message.offset };
    if (!message.value) {
      this.logger.warn(position, 'skipping ShipmentBooked message without a value');
      return;
    }

    let raw: unknown;
    try {
      raw = JSON.parse(message.value.toString('utf8'));
    } catch {
      this.logger.warn(position, 'skipping ShipmentBooked message that is not JSON');
      return;
    }

    const parsed = shipmentBookedEnvelopeSchema.safeParse(raw);
    if (!parsed.success) {
      this.logger.warn({ ...position, issues: parsed.error.issues }, 'rejected malformed ShipmentBooked');
      return;
    }

    const event = parsed.data.data;
    const request = await this.prisma.pickupRequest.upsert({
      where: { shipmentId: event.shipmentId },
      create: toPickupRequest(event, this.settings),
      update: {},
    });

    this.logger.info(
      {
        ...position,
        eventId: parsed.data.id,
        shipmentId: event.shipmentId,
        pickupRequestId: request.id,
        pickupDate: request.pickupDate.toISOString().slice(0, 10),
        serviceLevel: event.serviceLevel,
      },
      'pickup queued',
    );
  }
}
