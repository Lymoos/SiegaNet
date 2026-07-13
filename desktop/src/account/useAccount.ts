import { useCallback, useState } from "react";
import { accountApi, clearSession, hasActiveSubscription, loadSession, saveSession } from "./index";
import type { Account } from "./types";

export interface AccountStore {
  account: Account | null;
  loggedIn: boolean;
  hasSub: boolean;
  login: (email: string, password: string) => Promise<string | null>;
  logout: () => void;
  redeem: (code: string) => Promise<string | null>;
}

/**
 * Account/subscription state for the whole app. Persists the session so the
 * user isn't asked to log in on every launch; folds redeem results into the
 * cached account. Returns `null` on success and an error string on failure
 * for the login/redeem actions.
 */
export function useAccount(): AccountStore {
  const [session, setSession] = useState(loadSession);
  const account = session?.account ?? null;

  const login = useCallback(async (email: string, password: string) => {
    const res = await accountApi().login(email, password);
    if (!res.ok || !res.account) return res.error ?? "Не удалось войти";
    const s = { token: res.token, account: res.account };
    saveSession(s);
    setSession(s);
    return null;
  }, []);

  const logout = useCallback(() => {
    void accountApi().logout();
    clearSession();
    setSession(null);
  }, []);

  const redeem = useCallback(
    async (code: string) => {
      if (!session) return "Сначала войдите в аккаунт";
      const res = await accountApi().redeem(session.token, code);
      if (!res.ok) return res.error ?? "Код не подошёл";
      const next = {
        ...session,
        account: {
          ...session.account,
          subscription: { active: true, plan: res.plan, valid_until: res.valid_until },
        },
      };
      saveSession(next);
      setSession(next);
      return null;
    },
    [session],
  );

  return {
    account,
    loggedIn: session !== null,
    hasSub: hasActiveSubscription(account),
    login,
    logout,
    redeem,
  };
}
