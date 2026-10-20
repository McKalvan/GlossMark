import { STATIONS } from "../stations";
import type { Station } from "../types";

export function renderStationTabs(root: HTMLElement, active: Station): void {
  root.replaceChildren(
    ...STATIONS.map((s) => {
      const a = document.createElement("a");
      a.href = `#${s.id}`;
      a.textContent = s.label;
      a.style.borderColor = s.colour;
      if (s.id === active) a.setAttribute("aria-current", "page");
      return a;
    }),
  );
}
