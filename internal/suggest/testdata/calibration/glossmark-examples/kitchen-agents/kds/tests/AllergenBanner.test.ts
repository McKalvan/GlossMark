import { describe, expect, it } from "vitest";
import { renderAllergenBanner } from "../src/views/AllergenBanner";
import type { KitchenTicket } from "../src/types";

const base: KitchenTicket = {
  id: "t1", order_id: "o1", station: "grill", table: "T12", lines: [], allergen_alert: [],
  fired_at: "2026-10-19T19:00:00Z", bumped_at: null,
};

// @gloss verifies .AllergenBanner conf=0.9 scope=block
describe("AllergenBanner", () => {
  it("pins flagged tickets", () => {
    const root = document.createElement("section");
    renderAllergenBanner(root, [base, { ...base, id: "t2", allergen_alert: ["peanuts", "soya"] }]);
    expect(root.hidden).toBe(false);
    expect(root.textContent).toBe("ALLERGY T12: Peanuts, Soya");
  });

  it("hides when nothing is flagged", () => {
    const root = document.createElement("section");
    renderAllergenBanner(root, [base]);
    expect(root.hidden).toBe(true);
  });
});
