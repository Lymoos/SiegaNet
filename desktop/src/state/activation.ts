/**
 * Cached activation (subscription VPN): the key is entered once, verified via
 * POST /activate and cached with its expiry so the app doesn't ask again on
 * every launch. Without a valid activation the client refuses to connect.
 */

export interface Activation {
  key: string;
  /** unix seconds */
  validUntil: number;
}

const KEY = "sieganet.activation.v1";

export function loadActivation(): Activation | null {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return null;
    const a = JSON.parse(raw) as Activation;
    return typeof a.key === "string" && typeof a.validUntil === "number" ? a : null;
  } catch {
    return null;
  }
}

export function saveActivation(a: Activation): void {
  localStorage.setItem(KEY, JSON.stringify(a));
}

export function clearActivation(): void {
  localStorage.removeItem(KEY);
}

export function isActivated(): boolean {
  const a = loadActivation();
  return a !== null && a.validUntil > Math.floor(Date.now() / 1000);
}
