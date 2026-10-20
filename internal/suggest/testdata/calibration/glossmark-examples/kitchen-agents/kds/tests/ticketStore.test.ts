import { describe, expect, it } from "vitest";
import { TicketStore, minutesSinceFired } from "../src/ticketStore";
import type { KitchenTicket } from "../src/types";

function ticket(id: string, firedAt: string, station: KitchenTicket["station"] = "grill"): KitchenTicket {
  return { id, order_id: "o1", station, table: "T1", lines: [], allergen_alert: [], fired_at: firedAt, bumped_at: null };
}

// @gloss verifies .TicketStore conf=0.9 scope=block
describe("TicketStore", () => {
  it("keeps only its own station, oldest first", () => {
    const store = new TicketStore("grill");
    store.load([ticket("b", "2026-10-19T19:05:00Z"), ticket("a", "2026-10-19T19:01:00Z"), ticket("c", "2026-10-19T19:00:00Z", "fry")]);
    expect(store.open().map((t) => t.id)).toEqual(["a", "b"]);
  });

  it("drops a bumped ticket", () => {
    const store = new TicketStore("grill");
    store.load([ticket("a", "2026-10-19T19:01:00Z")]);
    store.receive({ ...ticket("a", "2026-10-19T19:01:00Z"), bumped_at: "2026-10-19T19:09:00Z" });
    expect(store.open()).toEqual([]);
  });

  it("counts whole minutes since firing", () => {
    expect(minutesSinceFired(ticket("a", "2026-10-19T19:00:00Z"), new Date("2026-10-19T19:12:59Z"))).toBe(12);
  });
});
