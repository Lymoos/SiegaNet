/**
 * Country name -> flag emoji, UI-side convenience.
 *
 * The control contract only carries a human-readable `country`; if the core
 * later adds an ISO country code to GET /servers (nice-to-have), this lookup
 * collapses to a two-codepoint transform.
 */

const NAME_TO_ISO: Record<string, string> = {
  "Нидерланды": "NL",
  "Германия": "DE",
  "Финляндия": "FI",
  "Швеция": "SE",
  "Великобритания": "GB",
  "Франция": "FR",
  "Польша": "PL",
  "Турция": "TR",
  "Казахстан": "KZ",
  "ОАЭ": "AE",
  "Сингапур": "SG",
  "Япония": "JP",
  "США": "US",
  "Бразилия": "BR",
};

export function flagFor(country: string): string {
  const iso = NAME_TO_ISO[country];
  if (!iso) return "🌐";
  const base = 0x1f1e6; // regional indicator A
  return String.fromCodePoint(
    base + iso.charCodeAt(0) - 65,
    base + iso.charCodeAt(1) - 65,
  );
}

/** two-letter ISO code for map pins; "??" for unknown countries */
export function isoFor(country: string): string {
  return NAME_TO_ISO[country] ?? "??";
}

/**
 * Load -> ring colour for map pins: green (free) → orange (busy) → red
 * (loaded), smooth over load_pct. Deliberately outside the base palette —
 * it's a data encoding, anchored on the theme's "connected" green.
 */
export function loadColor(loadPct: number): string {
  const t = Math.min(100, Math.max(0, loadPct));
  // hue: 145 (theme green) → 38 (orange) → 4 (red)
  const hue = t <= 50 ? 145 - ((145 - 38) * t) / 50 : 38 - ((38 - 4) * (t - 50)) / 50;
  return `hsl(${Math.round(hue)} 62% 56%)`;
}
