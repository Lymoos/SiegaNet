import { useEffect, useRef, useState } from "react";
import type { Account } from "../account/types";

interface Props {
  account: Account;
  /** open with the redeem field focused (from the subscription dialog) */
  focusRedeem?: boolean;
  onRedeem: (code: string) => Promise<string | null>;
  onLogout: () => void;
  onClose: () => void;
}

/**
 * Account profile slide-over: email, subscription status, subscription-code
 * redemption (moved here from the old pre-app gate — codes now live inside
 * the account), and logout.
 */
export function ProfilePanel({ account, focusRedeem, onRedeem, onLogout, onClose }: Props) {
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [justRedeemed, setJustRedeemed] = useState(false);
  const codeRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (focusRedeem) codeRef.current?.focus();
  }, [focusRedeem]);

  const sub = account.subscription;
  const active = sub.active && sub.valid_until > Math.floor(Date.now() / 1000);

  const redeem = async () => {
    if (busy || !code.trim()) return;
    setBusy(true);
    setError(null);
    const err = await onRedeem(code);
    setBusy(false);
    if (err) {
      setError(err);
    } else {
      setCode("");
      setJustRedeemed(true);
      setTimeout(() => setJustRedeemed(false), 2500);
    }
  };

  const initial = account.email.charAt(0).toUpperCase();

  return (
    <>
      <div className="settings-scrim" onClick={onClose} />
      <div className="settings-panel profile-panel">
        <div className="profile-head">
          <div className="profile-avatar">{initial}</div>
          <div style={{ minWidth: 0 }}>
            <div className="profile-email">{account.email}</div>
            <div className="profile-plan">
              <span className={`plan-dot ${active ? "on" : ""}`} />
              {active ? `Подписка ${sub.plan}` : "Без подписки"}
            </div>
          </div>
        </div>

        <div className={`sub-card ${active ? "active" : ""}`}>
          {active ? (
            <>
              <div className="sub-card-label">Подписка активна</div>
              <div className="sub-card-until">
                действует до{" "}
                {new Date(sub.valid_until * 1000).toLocaleDateString("ru-RU", {
                  day: "numeric",
                  month: "long",
                  year: "numeric",
                })}
              </div>
            </>
          ) : (
            <>
              <div className="sub-card-label">Подписка не активна</div>
              <div className="sub-card-until">
                Введите код подписки ниже или продлите доступ.
              </div>
            </>
          )}
        </div>

        <div className="set-group">
          <div className="set-label">Код подписки</div>
          <input
            ref={codeRef}
            className="activation-input"
            style={{ textAlign: "left" }}
            placeholder="XXXX-XXXX-XXXX-XXXX"
            value={code}
            onChange={(e) => setCode(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && void redeem()}
            disabled={busy}
          />
          {error && <div className="activation-error">{error}</div>}
          {justRedeemed && <div className="redeem-ok">Код применён ✓</div>}
          <button
            className="reconnect-btn"
            style={{ marginTop: 12 }}
            disabled={busy || !code.trim()}
            onClick={() => void redeem()}
          >
            {busy ? "Проверяем…" : "Применить код"}
          </button>
        </div>

        <button className="profile-logout" onClick={onLogout}>
          Выйти из аккаунта
        </button>
      </div>
    </>
  );
}
