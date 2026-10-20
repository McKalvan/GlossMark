pub mod publisher;
pub mod types;

pub const TOPIC_SHIPMENT_PICKED_UP: &str = "shipment.picked-up.v1";
pub const TOPIC_SHIPMENT_DELIVERED: &str = "shipment.delivered.v1";
pub const TOPIC_DISPATCH_ROUTES: &str = "dispatch.routes.v1";

pub const SHIPMENT_PICKED_UP: &str = "ShipmentPickedUp";
pub const SHIPMENT_DELIVERED: &str = "ShipmentDelivered";
pub const ROUTE_PLANNED: &str = "RoutePlanned";
