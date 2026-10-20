// @gloss represents dine:Station conf=0.95
export type Station = "grill" | "fry" | "saute" | "cold" | "pastry" | "pass";

// @gloss represents dine:Allergen conf=0.95
export type Allergen =
  | "celery" | "gluten" | "crustaceans" | "eggs" | "fish" | "lupin" | "milk"
  | "molluscs" | "mustard" | "tree_nuts" | "peanuts" | "sesame" | "soya" | "sulphites";

export interface TicketLine {
  name: string;
  quantity: number;
  modifiers: string[];
  seat: number | null;
}

// @gloss represents dine:KitchenTicket conf=0.95
export interface KitchenTicket {
  id: string;
  order_id: string;
  station: Station;
  table: string;
  lines: TicketLine[];
  allergen_alert: Allergen[];
  fired_at: string;
  bumped_at: string | null;
}
