import { bumpTicket, fetchOpenTickets, subscribeToTickets } from "./api";
import { stationFromHash } from "./stations";
import { TicketStore } from "./ticketStore";
import { renderAllergenBanner } from "./views/AllergenBanner";
import { renderStationTabs } from "./views/StationTabs";
import { renderTicketBoard } from "./views/TicketBoard";

const locationId = new URLSearchParams(location.search).get("location") ?? "";

function el(id: string): HTMLElement {
  const node = document.getElementById(id);
  if (!node) throw new Error(`missing #${id}`);
  return node;
}

// @gloss implements .KitchenDisplay conf=0.9
async function start(): Promise<void> {
  const station = stationFromHash(location.hash);
  const store = new TicketStore(station);
  renderStationTabs(el("stations"), station);
  store.subscribe((tickets) => {
    renderAllergenBanner(el("allergens"), tickets);
    renderTicketBoard(el("board"), tickets, async (id) => {
      store.bump(id);
      await bumpTicket(id);
    });
  });
  store.load(await fetchOpenTickets(locationId, station));
  subscribeToTickets((t) => store.receive(t));
  window.addEventListener("hashchange", () => location.reload());
}

void start();
