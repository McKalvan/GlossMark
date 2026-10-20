import type { PrismaClient } from '@prisma/client';
import type { FastifyPluginAsync } from 'fastify';
import { z } from 'zod';
import {
  ConfirmationError,
  type ConfirmationErrorCode,
  type PickupConfirmationService,
} from '../confirm/pickupConfirmation.js';
import { TRACKING_NUMBER } from '../consumer/schemas.js';
import { ISO_DATE, toDbDate } from '../time.js';

const STATUS_BY_CODE: Record<ConfirmationErrorCode, number> = {
  NOT_FOUND: 404,
  NOT_PLANNED: 409,
  WRONG_DRIVER: 403,
  TRACKING_MISMATCH: 422,
};

const confirmParams = z.object({ id: z.string().uuid() });

const confirmBody = z.object({
  driverId: z.string().uuid(),
  trackingNumber: z.string().regex(TRACKING_NUMBER),
  lat: z.number().min(-90).max(90),
  lng: z.number().min(-180).max(180),
  confirmedAt: z.string().datetime({ offset: true }).optional(),
});

const listQuery = z.object({
  date: z.string().regex(ISO_DATE),
  status: z.enum(['QUEUED', 'PLANNED', 'PICKED_UP', 'CANCELLED']).optional(),
});

export interface PickupsApiDeps {
  prisma: PrismaClient;
  confirmation: PickupConfirmationService;
}

export function pickupsApi({ prisma, confirmation }: PickupsApiDeps): FastifyPluginAsync {
  return async (app) => {
    // Called by the driver app after scanning the ShippingLabel at the shipper's door.
    app.post('/pickups/:id/confirm', async (req, reply) => {
      const params = confirmParams.safeParse(req.params);
      if (!params.success) {
        return reply.code(400).send({ error: 'INVALID_REQUEST', issues: params.error.issues });
      }
      const body = confirmBody.safeParse(req.body);
      if (!body.success) {
        return reply.code(400).send({ error: 'INVALID_REQUEST', issues: body.error.issues });
      }

      try {
        const result = await confirmation.confirm(params.data.id, {
          ...body.data,
          confirmedAt: body.data.confirmedAt ? new Date(body.data.confirmedAt) : new Date(),
        });
        return reply.code(200).send(result);
      } catch (err) {
        if (err instanceof ConfirmationError) {
          return reply.code(STATUS_BY_CODE[err.code]).send({ error: err.code, message: err.message });
        }
        throw err;
      }
    });

    app.get('/pickups', async (req, reply) => {
      const query = listQuery.safeParse(req.query);
      if (!query.success) {
        return reply.code(400).send({ error: 'INVALID_REQUEST', issues: query.error.issues });
      }

      const requests = await prisma.pickupRequest.findMany({
        where: { pickupDate: toDbDate(query.data.date), status: query.data.status },
        include: { stop: { select: { routeId: true, seq: true, etaFrom: true, etaTo: true } } },
        orderBy: [{ zone: 'asc' }, { bookedAt: 'asc' }],
      });

      return {
        date: query.data.date,
        pickups: requests.map((r) => ({
          id: r.id,
          shipmentId: r.shipmentId,
          trackingNumber: r.trackingNumber,
          serviceLevel: r.serviceLevel,
          status: r.status,
          zone: r.zone,
          address: { line1: r.line1, city: r.city, postcode: r.postcode, country: r.country },
          parcelCount: r.parcelCount,
          totalWeightGrams: r.totalWeightGrams,
          route: r.stop,
          pickedUpAt: r.pickedUpAt,
        })),
      };
    });
  };
}
