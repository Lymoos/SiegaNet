import { useState } from "react";

interface Props {
  onLogin: (email: string, password: string) => Promise<string | null>;
}

/**
 * Account gate: the app requires a SiegaNet account. Subscription state is
 * handled separately, inside the profile — logging in is enough to reach the
 * app; connecting is what a subscription unlocks.
 */
export function LoginScreen({ onLogin }: Props) {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async () => {
    if (busy || !email.trim() || !password) return;
    setBusy(true);
    setError(null);
    const err = await onLogin(email, password);
    setBusy(false);
    if (err) setError(err);
  };

  return (
    <div className="auth">
      <div className="auth-aurora" />
      <div className="auth-card">
        <div className="auth-brand">
          <div className="brand-glyph auth-glyph">
            <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M12 2 4 5.5v5.7c0 5 3.4 8.6 8 10.3 4.6-1.7 8-5.3 8-10.3V5.5L12 2Z" />
            </svg>
          </div>
          <div className="brand-name" style={{ fontSize: 24 }}>SiegaNet</div>
          <div className="brand-sub">stealth vpn</div>
        </div>

        <div className="auth-title">Вход в аккаунт</div>

        <label className="auth-label">Почта</label>
        <input
          className="auth-input"
          type="email"
          placeholder="you@example.com"
          value={email}
          autoFocus
          onChange={(e) => setEmail(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && void submit()}
          disabled={busy}
        />

        <label className="auth-label">Пароль</label>
        <input
          className="auth-input"
          type="password"
          placeholder="••••••••"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && void submit()}
          disabled={busy}
        />

        {error && <div className="activation-error">{error}</div>}

        <button
          className={`auth-submit ${busy ? "busy" : ""}`}
          disabled={busy || !email.trim() || !password}
          onClick={() => void submit()}
        >
          {busy ? "Входим…" : "Войти"}
        </button>

        <div className="auth-hint">
          Нет аккаунта? Регистрация появится в ближайшем обновлении.
        </div>
      </div>
    </div>
  );
}
