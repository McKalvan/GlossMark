-- CreateEnum
CREATE TYPE "ServiceLevel" AS ENUM ('STANDARD', 'EXPRESS');

-- CreateEnum
CREATE TYPE "PickupStatus" AS ENUM ('QUEUED', 'PLANNED', 'PICKED_UP', 'CANCELLED');

-- CreateEnum
CREATE TYPE "RouteStatus" AS ENUM ('PLANNED', 'IN_PROGRESS', 'COMPLETED');

-- CreateTable
CREATE TABLE "drivers" (
    "id" UUID NOT NULL,
    "employee_no" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "phone" TEXT NOT NULL,
    "zones" TEXT[],
    "shift_start" TEXT NOT NULL,
    "shift_end" TEXT NOT NULL,
    "active" BOOLEAN NOT NULL DEFAULT true,
    "vehicle_id" UUID,
    "created_at" TIMESTAMPTZ(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMPTZ(3) NOT NULL,

    CONSTRAINT "drivers_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "vehicles" (
    "id" UUID NOT NULL,
    "registration" TEXT NOT NULL,
    "payload_grams" INTEGER NOT NULL,
    "max_parcels" INTEGER NOT NULL,
    "created_at" TIMESTAMPTZ(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT "vehicles_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "pickup_requests" (
    "id" UUID NOT NULL,
    "shipment_id" TEXT NOT NULL,
    "tracking_number" TEXT NOT NULL,
    "shipper_id" TEXT NOT NULL,
    "service_level" "ServiceLevel" NOT NULL,
    "status" "PickupStatus" NOT NULL DEFAULT 'QUEUED',
    "pickup_date" DATE NOT NULL,
    "zone" TEXT NOT NULL,
    "address_line1" TEXT NOT NULL,
    "city" TEXT NOT NULL,
    "postcode" TEXT NOT NULL,
    "country" CHAR(2) NOT NULL,
    "lat" DOUBLE PRECISION NOT NULL,
    "lng" DOUBLE PRECISION NOT NULL,
    "parcel_count" INTEGER NOT NULL,
    "total_weight_grams" INTEGER NOT NULL,
    "delivery_window_start" TIMESTAMPTZ(3) NOT NULL,
    "delivery_window_end" TIMESTAMPTZ(3) NOT NULL,
    "booked_at" TIMESTAMPTZ(3) NOT NULL,
    "picked_up_at" TIMESTAMPTZ(3),
    "picked_up_by" UUID,
    "created_at" TIMESTAMPTZ(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMPTZ(3) NOT NULL,

    CONSTRAINT "pickup_requests_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "routes" (
    "id" UUID NOT NULL,
    "driver_id" UUID NOT NULL,
    "date" DATE NOT NULL,
    "status" "RouteStatus" NOT NULL DEFAULT 'PLANNED',
    "distance_km" DOUBLE PRECISION NOT NULL,
    "planned_at" TIMESTAMPTZ(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TIMESTAMPTZ(3) NOT NULL,

    CONSTRAINT "routes_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "route_stops" (
    "id" UUID NOT NULL,
    "route_id" UUID NOT NULL,
    "pickup_request_id" UUID NOT NULL,
    "seq" INTEGER NOT NULL,
    "eta_from" TIMESTAMPTZ(3) NOT NULL,
    "eta_to" TIMESTAMPTZ(3) NOT NULL,
    "completed_at" TIMESTAMPTZ(3),

    CONSTRAINT "route_stops_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "drivers_employee_no_key" ON "drivers"("employee_no");

-- CreateIndex
CREATE UNIQUE INDEX "drivers_vehicle_id_key" ON "drivers"("vehicle_id");

-- CreateIndex
CREATE UNIQUE INDEX "vehicles_registration_key" ON "vehicles"("registration");

-- CreateIndex
CREATE UNIQUE INDEX "pickup_requests_shipment_id_key" ON "pickup_requests"("shipment_id");

-- CreateIndex
CREATE INDEX "pickup_requests_pickup_date_status_idx" ON "pickup_requests"("pickup_date", "status");

-- CreateIndex
CREATE INDEX "routes_date_status_idx" ON "routes"("date", "status");

-- CreateIndex
CREATE UNIQUE INDEX "routes_driver_id_date_key" ON "routes"("driver_id", "date");

-- CreateIndex
CREATE UNIQUE INDEX "route_stops_pickup_request_id_key" ON "route_stops"("pickup_request_id");

-- CreateIndex
CREATE UNIQUE INDEX "route_stops_route_id_seq_key" ON "route_stops"("route_id", "seq");

-- AddForeignKey
ALTER TABLE "drivers" ADD CONSTRAINT "drivers_vehicle_id_fkey" FOREIGN KEY ("vehicle_id") REFERENCES "vehicles"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "routes" ADD CONSTRAINT "routes_driver_id_fkey" FOREIGN KEY ("driver_id") REFERENCES "drivers"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "route_stops" ADD CONSTRAINT "route_stops_route_id_fkey" FOREIGN KEY ("route_id") REFERENCES "routes"("id") ON DELETE CASCADE ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "route_stops" ADD CONSTRAINT "route_stops_pickup_request_id_fkey" FOREIGN KEY ("pickup_request_id") REFERENCES "pickup_requests"("id") ON DELETE RESTRICT ON UPDATE CASCADE;
