import type { AccountApi, LoginResult, RedeemResult } from "./types";

/**
 * Fully self-contained mock of the account backend.
 *
 * - login: any syntactically-valid email + non-empty password succeeds and
 *   returns a fresh account with NO active subscription (so the redeem flow
 *   is visible). The real backend replaces this with a credential check.
 * - redeem: any non-empty code activates a 30-day "Pro" subscription.
 *
 * The mock is stateless across calls — the caller (useAccount) persists the
 * account and folds redeem results into it, exactly as it will with the real
 * service.
 */
export class MockAccountApi implements AccountApi {
  async login(email: string, password: string): Promise<LoginResult> {
    await delay(650);
    const emailOk = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim());
    if (!emailOk) {
      return { ok: false, token: "", account: null, error: "Неверный формат почты" };
    }
    if (password.length < 4) {
      return { ok: false, token: "", account: null, error: "Пароль слишком короткий" };
    }
    return {
      ok: true,
      token: `mock-${btoa(email.trim()).replace(/=/g, "")}`,
      account: {
        email: email.trim(),
        subscription: { active: false, plan: "—", valid_until: 0 },
      },
    };
  }

  async logout(): Promise<void> {
    await delay(150);
  }

  async redeem(_token: string, code: string): Promise<RedeemResult> {
    await delay(700);
    if (code.trim().length === 0) {
      return { ok: false, valid_until: 0, plan: "—", error: "Введите код" };
    }
    return {
      ok: true,
      valid_until: Math.floor(Date.now() / 1000) + 30 * 24 * 3600,
      plan: "Pro",
    };
  }
}

function delay(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms + Math.random() * 300));
}
