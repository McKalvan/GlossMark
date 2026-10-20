export const TOPICS = {
  shipmentBooked: 'shipment.booked.v1',
  shipmentPickedUp: 'shipment.picked-up.v1',
  shipmentDelivered: 'shipment.delivered.v1',
  routePlanned: 'dispatch.routes.v1',
} as const;

export type Topic = (typeof TOPICS)[keyof typeof TOPICS];

export type EventType = 'ShipmentBooked' | 'ShipmentPickedUp' | 'ShipmentDelivered' | 'RoutePlanned';

/** CloudEvents-style envelope shared by every Kestrel Freight topic. */
export interface EventEnvelope<TType extends EventType, TData> {
  specversion: '1.0';
  id: string;
  type: TType;
  source: string;
  time: string;
  datacontenttype: 'application/json';
  data: TData;
}

export interface Address {
  line1: string;
  city: string;
  postcode: string;
  country: string;
  lat: number;
  lng: number;
}

export interface Parcel {
  parcelId: string;
  weightGrams: number;
  lengthCm: number;
  widthCm: number;
  heightCm: number;
}

export type ServiceLevel = 'STANDARD' | 'EXPRESS';

/** Published by shipment-core on shipment.booked.v1; consumed here. */
export interface ShipmentBooked {
  shipmentId: string;
  trackingNumber: string;
  shipperId: string;
  pickupAddress: Address;
  deliveryAddress: Address;
  parcels: Parcel[];
  serviceLevel: ServiceLevel;
  carrierCode: string;
  deliveryWindow: { start: string; end: string };
  bookedAt: string;
}

/** Published by dispatch on shipment.picked-up.v1 when the driver app confirms a pickup. */
// @gloss represents frt:ShipmentPickedUp conf=0.95
export interface ShipmentPickedUp {
  shipmentId: string;
  trackingNumber: string;
  driverId: string;
  routeId: string;
  pickedUpAt: string;
  lat: number;
  lng: number;
}

/** Published by tracking on shipment.delivered.v1. Dispatch does not consume it. */
export interface ShipmentDelivered {
  shipmentId: string;
  trackingNumber: string;
  deliveredAt: string;
  lat: number;
  lng: number;
  proof: { kind: 'SIGNATURE' | 'PHOTO' | 'GEOFENCE'; ref: string };
  withinWindow: boolean;
}

export interface RoutePlannedStop {
  shipmentId: string;
  seq: number;
  lat: number;
  lng: number;
  etaFrom: string;
  etaTo: string;
}

/** Published by dispatch on dispatch.routes.v1 whenever a route is planned or changed. */
export interface RoutePlanned {
  routeId: string;
  driverId: string;
  date: string;
  stops: RoutePlannedStop[];
}
