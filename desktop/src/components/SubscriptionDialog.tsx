interface Props {
  onRenew: () => void;
  onUseCode: () => void;
  onClose: () => void;
}

/**
 * Shown when the user hits connect without an active subscription. Explains
 * that connecting isn't available, offers to renew, and — per the brief — a
 * "Воспользоваться кодом" link that jumps straight to code entry in profile.
 */
export function SubscriptionDialog({ onRenew, onUseCode, onClose }: Props) {
  return (
    <div className="dialog-scrim" onClick={onClose}>
      <div className="dialog" onClick={(e) => e.stopPropagation()}>
        <div className="dialog-glyph">
          <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
            <rect x="3" y="11" width="18" height="10" rx="2" />
            <path d="M7 11V8a5 5 0 0 1 10 0v3" />
          </svg>
        </div>
        <div className="dialog-title">Нужна активная подписка</div>
        <div className="dialog-text">
          Подключение недоступно без активной подписки. Продлите доступ или
          воспользуйтесь кодом подписки.
        </div>
        <button className="dialog-primary" onClick={onRenew}>
          Продлить подписку
        </button>
        <button className="dialog-link" onClick={onUseCode}>
          Воспользоваться кодом
        </button>
      </div>
    </div>
  );
}
