import type { KitchenTicket, Station } from "./types";

const BASE = "/api";

export async function fetchOpenTickets(locationId: string, station: Station): Promise<KitchenTicket[]> {
  const res = await fetch(`${BASE}/tickets/${locationId}/${station}`);
  if (!res.ok) throw new Error(`tickets: ${res.status}`);
  return (await res.json()) as KitchenTicket[];
}

export async function bumpTicket(ticketId: string): Promise<void> {
  const res = await fetch(`${BASE}/tickets/${ticketId}/bump`, { method: "POST" });
  if (!res.ok && res.status !== 404) throw new Error(`bump: ${res.status}`);
}

export function subscribeToTickets(onTicket: (t: KitchenTicket) => void): () => void {
  const source = new EventSource(`${BASE}/tickets/stream`);
  source.onmessage = (ev) => onTicket(JSON.parse(ev.data) as KitchenTicket);
  return () => source.close();
}
