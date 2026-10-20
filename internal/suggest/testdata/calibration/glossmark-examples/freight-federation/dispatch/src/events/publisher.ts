import { randomUUID } from 'node:crypto';
import type { Producer } from 'kafkajs';
import type { BaseLogger } from 'pino';
import { fromDbDate } from '../time.js';
import {
  TOPICS,
  type EventEnvelope,
  type EventType,
  type RoutePlanned,
  type ShipmentPickedUp,
  type Topic,
} from './types.js';

const SOURCE = 'urn:kestrel:dispatch';

export interface RouteForEvent {
  id: string;
  driverId: string;
  date: Date;
  stops: ReadonlyArray<{
    seq: number;
    etaFrom: Date;
    etaTo: Date;
    pickupRequest: { shipmentId: string; lat: number; lng: number };
  }>;
}

export function buildRoutePlanned(route: RouteForEvent): RoutePlanned {
  return {
    routeId: route.id,
    driverId: route.driverId,
    date: fromDbDate(route.date),
    stops: [...route.stops]
      .sort((a, b) => a.seq - b.seq)
      .map((stop) => ({
        shipmentId: stop.pickupRequest.shipmentId,
        seq: stop.seq,
        lat: stop.pickupRequest.lat,
        lng: stop.pickupRequest.lng,
        etaFrom: stop.etaFrom.toISOString(),
        etaTo: stop.etaTo.toISOString(),
      })),
  };
}

export class EventPublisher {
  constructor(
    private readonly producer: Producer,
    private readonly logger: BaseLogger,
  ) {}

  // @gloss publishes frt:ShipmentPickedUp conf=0.9
  publishShipmentPickedUp(data: ShipmentPickedUp): Promise<void> {
    return this.send(TOPICS.shipmentPickedUp, 'ShipmentPickedUp', data.shipmentId, data);
  }

  publishRoutePlanned(data: RoutePlanned): Promise<void> {
    return this.send(TOPICS.routePlanned, 'RoutePlanned', data.routeId, data);
  }

  private async send<TType extends EventType, TData>(
    topic: Topic,
    type: TType,
    key: string,
    data: TData,
  ): Promise<void> {
    const envelope: EventEnvelope<TType, TData> = {
      specversion: '1.0',
      id: randomUUID(),
      type,
      source: SOURCE,
      time: new Date().toISOString(),
      datacontenttype: 'application/json',
      data,
    };

    await this.producer.send({
      topic,
      acks: -1,
      messages: [
        {
          key,
          value: JSON.stringify(envelope),
          headers: { ce_type: type, ce_id: envelope.id, ce_source: SOURCE },
        },
      ],
    });

    this.logger.info({ topic, type, key, eventId: envelope.id }, 'event published');
  }
}
