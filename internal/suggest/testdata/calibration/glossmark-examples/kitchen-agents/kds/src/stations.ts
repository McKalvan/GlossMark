import type { Station } from "./types";

export const STATIONS: { id: Station; label: string; colour: string }[] = [
  { id: "grill", label: "Grill", colour: "#c2410c" },
  { id: "fry", label: "Fry", colour: "#ca8a04" },
  { id: "saute", label: "Sauté", colour: "#0f766e" },
  { id: "cold", label: "Cold", colour: "#2563eb" },
  { id: "pastry", label: "Pastry", colour: "#9333ea" },
  { id: "pass", label: "Pass", colour: "#334155" },
];

export function stationFromHash(hash: string): Station {
  const id = hash.replace(/^#/, "");
  return STATIONS.find((s) => s.id === id)?.id ?? "pass";
}
