import { minutesSinceFired } from "../ticketStore";
import type { KitchenTicket } from "../types";

const LATE_AFTER_MIN = 12;

// @gloss implements .TicketBoard conf=0.55
export function renderTicketBoard(root: HTMLElement, tickets: KitchenTicket[], onBump: (id: string) => void): void {
  root.replaceChildren(...tickets.map((t) => ticketCard(t, onBump)));
}

function ticketCard(ticket: KitchenTicket, onBump: (id: string) => void): HTMLElement {
  const card = document.createElement("article");
  card.className = "ticket";
  card.dataset.ticketId = ticket.id;
  const age = minutesSinceFired(ticket);
  if (age >= LATE_AFTER_MIN) card.classList.add("late");
  if (ticket.allergen_alert.length > 0) card.classList.add("allergen");

  const header = document.createElement("header");
  header.textContent = `${ticket.table} · ${age} min`;
  const list = document.createElement("ul");
  for (const line of ticket.lines) {
    const li = document.createElement("li");
    li.textContent = `${line.quantity}× ${line.name}${line.modifiers.length ? ` (${line.modifiers.join(", ")})` : ""}`;
    list.append(li);
  }
  const bump = document.createElement("button");
  bump.textContent = "Bump";
  bump.addEventListener("click", () => onBump(ticket.id));
  card.append(header, list, bump);
  return card;
}
