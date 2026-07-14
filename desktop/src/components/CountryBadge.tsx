import { isoFor } from "../lib/flags";
import { flagUrlFor } from "../lib/flagAssets";

/**
 * Circular country flag. A real rectangular flag is cropped into a centred
 * circle (object-fit: cover) — it fills the badge and never overflows. The
 * ring (server load colour) hugs the circle as an outline, so the flag stays
 * full-bleed. Falls back to the ISO code only if a flag asset is missing.
 */
export function CountryBadge({
  country,
  size = 34,
  ring,
}: {
  country: string;
  size?: number;
  /** ring colour, e.g. load encoding; omitted => subtle theme outline */
  ring?: string;
}) {
  const url = flagUrlFor(country);
  const boxShadow = ring
    ? `0 0 0 2px ${ring}`
    : "0 0 0 1px var(--line-strong)";

  if (!url) {
    return (
      <span
        className="country-badge"
        style={{ width: size, height: size, fontSize: size * 0.34, boxShadow }}
        aria-label={country}
      >
        {isoFor(country)}
      </span>
    );
  }

  return (
    <span
      className="flag-badge"
      style={{ width: size, height: size, boxShadow }}
      aria-label={country}
      title={country}
    >
      <img src={url} alt="" draggable={false} />
    </span>
  );
}
