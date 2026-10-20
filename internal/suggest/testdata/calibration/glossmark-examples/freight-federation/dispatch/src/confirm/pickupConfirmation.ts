import type { PrismaClient } from '@prisma/client';
import type { BaseLogger } from 'pino';
import type { EventPublisher } from '../events/publisher.js';

export type ConfirmationErrorCode = 'NOT_FOUND' | 'NOT_PLANNED' | 'WRONG_DRIVER' | 'TRACKING_MISMATCH';

export class ConfirmationError extends Error {
  constructor(
    readonly code: ConfirmationErrorCode,
    message: string,
  ) {
    super(message);
    this.name = 'ConfirmationError';
  }
}

export interface ConfirmPickupInput {
  driverId: string;
  /** Scanned from the ShippingLabel barcode; must match the request. */
  trackingNumber: string;
  lat: number;
  lng: number;
  confirmedAt: Date;
}

export interface ConfirmPickupResult {
  pickupRequestId: string;
  shipmentId: string;
  routeId: string;
  pickedUpAt: Date;
  alreadyConfirmed: boolean;
}

// @gloss implements .PickupConfirmation conf=0.9
export class PickupConfirmationService {
  constructor(
    private readonly prisma: PrismaClient,
    private readonly publisher: Pick<EventPublisher, 'publishShipmentPickedUp'>,
    private readonly logger: BaseLogger,
  ) {}

  async confirm(pickupRequestId: string, input: ConfirmPickupInput): Promise<ConfirmPickupResult> {
    const result = await this.prisma.$transaction(async (tx) => {
      const request = await tx.pickupRequest.findUnique({
        where: { id: pickupRequestId },
        include: { stop: { include: { route: true } } },
      });
      if (!request) {
        throw new ConfirmationError('NOT_FOUND', `pickup request ${pickupRequestId} not found`);
      }
      if (request.trackingNumber !== input.trackingNumber) {
        throw new ConfirmationError(
          'TRACKING_MISMATCH',
          `scanned ${input.trackingNumber} does not match pickup request ${pickupRequestId}`,
        );
      }
      const stop = request.stop;
      if (!stop) {
        throw new ConfirmationError('NOT_PLANNED', `pickup request ${pickupRequestId} is not on a route`);
      }
      if (stop.route.driverId !== input.driverId) {
        throw new ConfirmationError(
          'WRONG_DRIVER',
          `route ${stop.route.id} belongs to another driver`,
        );
      }

      if (request.status === 'PICKED_UP' && request.pickedUpAt) {
        return {
          request,
          routeId: stop.route.id,
          pickedUpAt: request.pickedUpAt,
          alreadyConfirmed: true,
        };
      }
      if (request.status !== 'PLANNED') {
        throw new ConfirmationError(
          'NOT_PLANNED',
          `pickup request ${pickupRequestId} is ${request.status.toLowerCase()}`,
        );
      }

      const updated = await tx.pickupRequest.update({
        where: { id: request.id },
        data: { status: 'PICKED_UP', pickedUpAt: input.confirmedAt, pickedUpBy: input.driverId },
      });
      await tx.routeStop.update({
        where: { id: stop.id },
        data: { completedAt: input.confirmedAt },
      });

      const openStops = await tx.routeStop.count({
        where: { routeId: stop.route.id, completedAt: null },
      });
      const nextStatus = openStops === 0 ? 'COMPLETED' : 'IN_PROGRESS';
      if (stop.route.status !== nextStatus) {
        await tx.route.update({ where: { id: stop.route.id }, data: { status: nextStatus } });
      }

      return {
        request: updated,
        routeId: stop.route.id,
        pickedUpAt: input.confirmedAt,
        alreadyConfirmed: false,
      };
    });

    // Published after commit. A retried confirmation from the driver app republishes, so the
    // event is at-least-once; tracking and shipment-core deduplicate on shipmentId.
    await this.publisher.publishShipmentPickedUp({
      shipmentId: result.request.shipmentId,
      trackingNumber: result.request.trackingNumber,
      driverId: input.driverId,
      routeId: result.routeId,
      pickedUpAt: result.pickedUpAt.toISOString(),
      lat: input.lat,
      lng: input.lng,
    });

    this.logger.info(
      {
        pickupRequestId,
        shipmentId: result.request.shipmentId,
        routeId: result.routeId,
        alreadyConfirmed: result.alreadyConfirmed,
      },
      'pickup confirmed',
    );

    return {
      pickupRequestId,
      shipmentId: result.request.shipmentId,
      routeId: result.routeId,
      pickedUpAt: result.pickedUpAt,
      alreadyConfirmed: result.alreadyConfirmed,
    };
  }
}
