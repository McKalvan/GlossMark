import type { Allergen, KitchenTicket } from "../types";

const LABELS: Record<Allergen, string> = {
  celery: "Celery", gluten: "Gluten", crustaceans: "Crustaceans", eggs: "Eggs", fish: "Fish",
  lupin: "Lupin", milk: "Milk", molluscs: "Molluscs", mustard: "Mustard", tree_nuts: "Tree nuts",
  peanuts: "Peanuts", sesame: "Sesame", soya: "Soya", sulphites: "Sulphites",
};

/** Pins every open ticket with an allergen alert to the top of the screen until it is bumped. */
// @gloss implements .AllergenBanner conf=0.55
export function renderAllergenBanner(root: HTMLElement, tickets: KitchenTicket[]): void {
  const flagged = tickets.filter((t) => t.allergen_alert.length > 0);
  root.hidden = flagged.length === 0;
  root.replaceChildren(
    ...flagged.map((t) => {
      const row = document.createElement("p");
      row.className = "allergen-alert";
      row.textContent = `ALLERGY ${t.table}: ${t.allergen_alert.map((a) => LABELS[a]).join(", ")}`;
      return row;
    }),
  );
}
