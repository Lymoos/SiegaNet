import { useState } from "react";
import { controlApi } from "../api";
import { saveActivation } from "../state/activation";

interface Props {
  onActivated: () => void;
}

/**
 * Full-screen gate: the subscription model. Without a valid activation key
 * the client does not work at all — the key is verified via POST /activate
 * (mocked for now) and cached with its expiry.
 */
export function ActivationScreen({ onActivated }: Props) {
  const [key, setKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async () => {
    const trimmed = key.trim();
    if (!trimmed || busy) return;
    setBusy(true);
    setError(null);
    try {
      const res = await controlApi().activate(trimmed);
      if (res.ok) {
        saveActivation({ key: trimmed, validUntil: res.valid_until });
        onActivated();
      } else {
        setError("Ключ не подошёл. Проверьте и попробуйте ещё раз.");
      }
    } catch {
      setError("Не удалось проверить ключ. Попробуйте позже.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="activation">
      <div className="activation-card">
        <div className="brand" style={{ justifyContent: "center", marginBottom: 18 }}>
          <div className="brand-glyph">
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M12 2 4 5.5v5.7c0 5 3.4 8.6 8 10.3 4.6-1.7 8-5.3 8-10.3V5.5L12 2Z" />
            </svg>
          </div>
          <div>
            <div className="brand-name">SiegaNet</div>
            <div className="brand-sub">stealth vpn</div>
          </div>
        </div>

        <div className="activation-status">Требуется активация</div>
        <p className="activation-hint">
          Введите ключ активации подписки. Без него подключение недоступно.
        </p>

        <input
          className="activation-input"
          placeholder="XXXX-XXXX-XXXX-XXXX"
          value={key}
          autoFocus
          onChange={(e) => setKey(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") void submit();
          }}
          disabled={busy}
        />

        {error && <div className="activation-error">{error}</div>}

        <button
          className="reconnect-btn"
          style={{ marginTop: 14 }}
          disabled={busy || key.trim().length === 0}
          onClick={() => void submit()}
        >
          {busy ? "Проверяем…" : "Активировать"}
        </button>
      </div>
    </div>
  );
}
