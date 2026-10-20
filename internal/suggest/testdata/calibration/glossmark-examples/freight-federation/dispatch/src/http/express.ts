import type { FastifyPluginAsync } from 'fastify';
import { z } from 'zod';
import type { ExpressPickupOutcome, ExpressPickupService } from '../planning/expressPickup.js';

const STATUS_BY_OUTCOME: Record<ExpressPickupOutcome['outcome'], number> = {
  INSERTED: 201,
  QUEUED_FOR_PLANNING: 202,
  REJECTED_CUTOFF: 422,
  NO_ROUTE_AVAILABLE: 409,
  NOT_FOUND: 404,
  NOT_QUEUED: 409,
};

const expressBody = z.object({ shipmentId: z.string().min(1) });

export interface ExpressApiDeps {
  express: ExpressPickupService;
}

export function expressPickupsApi({ express }: ExpressApiDeps): FastifyPluginAsync {
  return async (app) => {
    // Requests same-day collection of an already-booked EXPRESS shipment.
    app.post('/express-pickups', async (req, reply) => {
      const body = expressBody.safeParse(req.body);
      if (!body.success) {
        return reply.code(400).send({ error: 'INVALID_REQUEST', issues: body.error.issues });
      }

      const result = await express.request(body.data.shipmentId);
      return reply.code(STATUS_BY_OUTCOME[result.outcome]).send(result);
    });
  };
}
