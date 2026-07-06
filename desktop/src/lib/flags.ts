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
