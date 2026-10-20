import Fastify from 'fastify';
import { Kafka, logLevel } from 'kafkajs';
import { loadConfig, planningSettings } from './config.js';
import { PickupConfirmationService } from './confirm/pickupConfirmation.js';
import { ShipmentBookedConsumer } from './consumer/shipmentBookedConsumer.js';
import { prisma } from './db.js';
import { EventPublisher } from './events/publisher.js';
import { expressPickupsApi } from './http/express.js';
import { pickupsApi } from './http/pickups.js';
import { routesApi } from './http/routes.js';
import { ExpressPickupService } from './planning/expressPickup.js';
import { PickupPlanner } from './planning/pickupPlanner.js';

async function main(): Promise<void> {
  const config = loadConfig();
  const app = Fastify({
    logger: { level: config.LOG_LEVEL, base: { service: 'dispatch' } },
  });
  const logger = app.log;

  const kafka = new Kafka({
    clientId: config.KAFKA_CLIENT_ID,
    brokers: config.KAFKA_BROKERS,
    logLevel: logLevel.WARN,
  });
  const producer = kafka.producer({ idempotent: true, maxInFlightRequests: 1 });
  await producer.connect();

  const settings = planningSettings(config);
  const publisher = new EventPublisher(producer, logger);
  const planner = new PickupPlanner(prisma, publisher, settings, logger);
  const express = new ExpressPickupService(prisma, publisher, settings, logger);
  const confirmation = new PickupConfirmationService(prisma, publisher, logger);
  const consumer = new ShipmentBookedConsumer(kafka, config.KAFKA_GROUP_ID, prisma, settings, logger);

  app.get('/healthz', async () => ({ status: 'ok' }));
  await app.register(pickupsApi({ prisma, confirmation }));
  await app.register(expressPickupsApi({ express }));
  await app.register(routesApi({ prisma, planner }));

  await consumer.start();
  await app.listen({ host: '0.0.0.0', port: config.PORT });

  let shuttingDown = false;
  const shutdown = async (signal: NodeJS.Signals): Promise<void> => {
    if (shuttingDown) return;
    shuttingDown = true;
    logger.info({ signal }, 'shutting down');
    await app.close();
    await consumer.stop();
    await producer.disconnect();
    await prisma.$disconnect();
    process.exit(0);
  };
  process.on('SIGTERM', (signal) => void shutdown(signal));
  process.on('SIGINT', (signal) => void shutdown(signal));
}

main().catch((err: unknown) => {
  console.error('dispatch failed to start', err);
  process.exit(1);
});
