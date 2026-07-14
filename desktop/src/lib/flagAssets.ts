import { isoFor } from "./flags";

/**
 * Real rectangular flags (flag-icons 4x3 SVGs), bundled as asset URLs. The
 * UI crops each into a centred circle (object-fit: cover), so the flag fills
 * the badge without overflowing — no emoji.
 */
const urls = import.meta.glob<string>("../assets/flags/*.svg", {
  eager: true,
  query: "?url",
  import: "default",
});

const byIso: Record<string, string> = {};
for (const [path, url] of Object.entries(urls)) {
  const iso = path.slice(path.lastIndexOf("/") + 1).replace(".svg", "");
  byIso[iso] = url;
}

/** flag asset URL for a country name, or null if we have no flag for it */
export function flagUrlFor(country: string): string | null {
  return byIso[isoFor(country).toLowerCase()] ?? null;
}
