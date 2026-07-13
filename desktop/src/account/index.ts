import { MockAccountApi } from "./mock";
import type { Account, AccountApi } from "./types";

/**
 * Single account-service client. Mock today; the real HTTP client drops in
 * here later behind the same AccountApi interface.
 */
let instance: AccountApi | null = null;

export function accountApi(): AccountApi {
  if (instance === null) instance = new MockAccountApi();
  return instance;
}

// ---- session persistence ----------------------------------------------------

export interface Session {
  token: string;
  account: Account;
}

const KEY = "sieganet.session.v1";

export function loadSession(): Session | null {
  try {
    const raw = localStorage.getItem(KEY);
    if (!raw) return null;
    const s = JSON.parse(raw) as Session;
    return s.token && s.account ? s : null;
  } catch {
    return null;
  }
}

export function saveSession(s: Session): void {
  localStorage.setItem(KEY, JSON.stringify(s));
}

export function clearSession(): void {
  localStorage.removeItem(KEY);
}

/** true when the cached account has a subscription that hasn't expired */
export function hasActiveSubscription(account: Account | null): boolean {
  if (!account) return false;
  const { subscription } = account;
  return subscription.active && subscription.valid_until > Math.floor(Date.now() / 1000);
}
