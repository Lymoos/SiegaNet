/**
 * Account / subscription contract (backend, not the local core).
 *
 * Accounts and subscriptions live on the SiegaNet backend, separate from the
 * tunnel core (127.0.0.1). The UI talks to this service for auth and for
 * redeeming subscription codes; it talks to the control API only for the
 * tunnel itself. Everything here is mocked for now (see account/mock.ts) and
 * swaps to real HTTP later without touching the UI.
 *
 *   POST /auth/login     { email, password } -> { ok, token, account }
 *   POST /auth/logout                        -> { ok }
 *   GET  /account                            -> Account
 *   POST /account/redeem { code }            -> { ok, valid_until, plan }
 */

export interface Subscription {
  active: boolean;
  /** display name of the plan, e.g. "Pro"; "—" when none */
  plan: string;
  /** unix seconds until which the subscription is valid; 0 when inactive */
  valid_until: number;
}

export interface Account {
  email: string;
  subscription: Subscription;
}

export interface LoginResult {
  ok: boolean;
  token: string;
  account: Account | null;
  /** human-readable reason when !ok */
  error?: string;
}

export interface RedeemResult {
  ok: boolean;
  valid_until: number;
  plan: string;
  error?: string;
}

export interface AccountApi {
  login(email: string, password: string): Promise<LoginResult>;
  logout(): Promise<void>;
  redeem(token: string, code: string): Promise<RedeemResult>;
}
