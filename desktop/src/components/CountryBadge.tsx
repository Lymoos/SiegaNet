import { isoFor } from "../lib/flags";

/**
 * Country badge that replaces the emoji flags (which rendered inconsistently
 * across platforms and looked cheap on the dark theme). A circular chip with
 * the 2-letter ISO code in mono — consistent with the map pins, premium on
 * near-black. An optional ring colour (server load) can be passed in.
 */
export function CountryBadge({
  country,
  size = 34,
  ring,
}: {
  country: string;
  size?: number;
  /** ring colour, e.g. load encoding; omitted => subtle theme border */
  ring?: string;
}) {
  return (
    <span
      className="country-badge"
      style={{
        width: size,
        height: size,
        fontSize: size * 0.34,
        borderColor: ring ?? undefined,
        borderWidth: ring ? 2 : 1,
      }}
      aria-label={country}
    >
      {isoFor(country)}
    </span>
  );
}
