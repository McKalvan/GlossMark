import type { KitchenTicket, Station } from "./types";

export type Listener = (tickets: KitchenTicket[]) => void;

/** Open tickets for one station, oldest first; the board renders whatever this holds. */
// @gloss implements .TicketStore conf=0.95
export class TicketStore {
  private tickets = new Map<string, KitchenTicket>();
  private listeners = new Set<Listener>();

  constructor(private readonly station: Station) {}

  load(tickets: KitchenTicket[]): void {
    this.tickets.clear();
    for (const t of tickets) if (t.station === this.station && !t.bumped_at) this.tickets.set(t.id, t);
    this.emit();
  }

  receive(ticket: KitchenTicket): void {
    if (ticket.station !== this.station) return;
    if (ticket.bumped_at) this.tickets.delete(ticket.id);
    else this.tickets.set(ticket.id, ticket);
    this.emit();
  }

  bump(ticketId: string): KitchenTicket | undefined {
    const t = this.tickets.get(ticketId);
    this.tickets.delete(ticketId);
    this.emit();
    return t;
  }

  open(): KitchenTicket[] {
    return [...this.tickets.values()].sort((a, b) => a.fired_at.localeCompare(b.fired_at));
  }

  subscribe(fn: Listener): () => void {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  }

  private emit(): void {
    const snapshot = this.open();
    for (const fn of this.listeners) fn(snapshot);
  }
}

export function minutesSinceFired(ticket: KitchenTicket, now: Date = new Date()): number {
  return Math.floor((now.getTime() - Date.parse(ticket.fired_at)) / 60_000);
}
