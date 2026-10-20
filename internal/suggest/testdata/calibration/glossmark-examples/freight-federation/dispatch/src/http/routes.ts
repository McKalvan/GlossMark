import type { PrismaClient } from '@prisma/client';
import type { FastifyPluginAsync } from 'fastify';
import { z } from 'zod';
import { PlanConflictError, type PickupPlanner } from '../planning/pickupPlanner.js';
import { ISO_DATE, fromDbDate, toDbDate } from '../time.js';

const dateQuery = z.object({ date: z.string().regex(ISO_DATE) });
const driverParams = z.object({ driverId: z.string().uuid() });

export interface RoutesApiDeps {
  prisma: PrismaClient;
  planner: PickupPlanner;
}

export function routesApi({ prisma, planner }: RoutesApiDeps): FastifyPluginAsync {
  return async (app) => {
    // The driver app loads its day's route from here.
    app.get('/routes/:driverId', async (req, reply) => {
      const params = driverParams.safeParse(req.params);
      const query = dateQuery.safeParse(req.query);
      if (!params.success || !query.success) {
        return reply.code(400).send({
          error: 'INVALID_REQUEST',
          issues: [...(params.error?.issues ?? []), ...(query.error?.issues ?? [])],
        });
      }

      const route = await prisma.route.findUnique({
        where: { driverId_date: { driverId: params.data.driverId, date: toDbDate(query.data.date) } },
        include: { stops: { include: { pickupRequest: true }, orderBy: { seq: 'asc' } } },
      });
      if (!route) {
        return reply.code(404).send({ error: 'NOT_FOUND', message: 'no route for driver on that date' });
      }

      return {
        routeId: route.id,
        driverId: route.driverId,
        date: fromDbDate(route.date),
        status: route.status,
        distanceKm: route.distanceKm,
        stops: route.stops.map((stop) => ({
          seq: stop.seq,
          pickupRequestId: stop.pickupRequestId,
          shipmentId: stop.pickupRequest.shipmentId,
          trackingNumber: stop.pickupRequest.trackingNumber,
          serviceLevel: stop.pickupRequest.serviceLevel,
          address: {
            line1: stop.pickupRequest.line1,
            city: stop.pickupRequest.city,
            postcode: stop.pickupRequest.postcode,
          },
          lat: stop.pickupRequest.lat,
          lng: stop.pickupRequest.lng,
          parcelCount: stop.pickupRequest.parcelCount,
          etaFrom: stop.etaFrom,
          etaTo: stop.etaTo,
          completedAt: stop.completedAt,
        })),
      };
    });

    // Triggered by the nightly scheduler (or an operator) for the next day.
    app.post('/routes/plan', async (req, reply) => {
      const query = dateQuery.safeParse(req.query);
      if (!query.success) {
        return reply.code(400).send({ error: 'INVALID_REQUEST', issues: query.error.issues });
      }

      try {
        const summary = await planner.planDay(query.data.date);
        return reply.code(201).send(summary);
      } catch (err) {
        if (err instanceof PlanConflictError) {
          return reply.code(409).send({ error: 'PLAN_EXISTS', message: err.message });
        }
        throw err;
      }
    });
  };
}
