import type { Server, Status } from "../api/types";
import type { Account } from "../account/types";
import { formatBytes, formatSince } from "../lib/format";

const STATE_LABEL: Record<Status["state"], string> = {
  disconnected: "Не подключено",
  connecting: "Подключение…",
  connected: "Защищено",
};

interface Props {
  status: Status;
  connectedServer: Server | null;
  target: Server | null;
  upRate: number;
  downRate: number;
  account: Account | null;
  hasSub: boolean;
  onConnect: (serverId: string) => void;
  onDisconnect: () => void;
  onOpenSettings: () => void;
  onOpenProfile: () => void;
}

export function StatusBar({
  status,
  connectedServer,
  target,
  upRate,
  downRate,
  account,
  hasSub,
  onConnect,
  onDisconnect,
  onOpenSettings,
  onOpenProfile,
}: Props) {
  const { state } = status;

  const where =
    state !== "disconnected" && connectedServer
      ? `${connectedServer.city}, ${connectedServer.country}`
      : state === "connecting" && target
        ? `${target.city}, ${target.country}`
        : null;

  const mainAction = () => {
    if (state === "disconnected") {
      if (target) onConnect(target.id);
    } else {
      onDisconnect();
    }
  };

  const mainLabel =
    state === "connected"
      ? "Отключиться"
      : state === "connecting"
        ? "Отмена"
        : "Подключиться";

  const initial = account?.email.charAt(0).toUpperCase() ?? "?";

  return (
    <header className="statusbar">
      <div className="state-pill">
        <span className={`state-dot ${state}`} />
        <span className="state-label">{STATE_LABEL[state]}</span>
      </div>

      {where && <span className="state-where">{where}</span>}

      <div className="stats">
        {state === "connected" && status.since_unix !== null && (
          <span className="stat">
            сессия <b>{formatSince(status.since_unix, Date.now())}</b>
          </span>
        )}
        {state === "connected" && (
          <>
            <span className="stat">
              <span className="up">↑</span> <b>{formatBytes(Math.round(upRate))}/с</b>{" "}
              · {formatBytes(status.up_bytes)}
            </span>
            <span className="stat">
              <span className="down">↓</span> <b>{formatBytes(Math.round(downRate))}/с</b>{" "}
              · {formatBytes(status.down_bytes)}
            </span>
          </>
        )}
        {status.inner_ip && <span className="ip-chip">{status.inner_ip}</span>}
      </div>

      <button
        className={`connect-btn ${state === "connected" ? "on" : ""} ${state === "connecting" ? "busy" : ""} ${!hasSub && state === "disconnected" ? "locked" : ""}`}
        onClick={mainAction}
        disabled={state === "disconnected" && !target}
        title={!hasSub && state === "disconnected" ? "Нужна активная подписка" : undefined}
      >
        {!hasSub && state === "disconnected" && (
          <svg className="lock-ico" width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2">
            <rect x="4" y="11" width="16" height="9" rx="2" />
            <path d="M8 11V8a4 4 0 0 1 8 0v3" />
          </svg>
        )}
        {mainLabel}
      </button>

      <button className="icon-btn" title="Настройки" onClick={onOpenSettings}>
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8">
          <circle cx="12" cy="12" r="3" />
          <path d="M19.4 15a1.7 1.7 0 0 0 .34 1.87l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.7 1.7 0 0 0-1.87-.34 1.7 1.7 0 0 0-1.03 1.56V21a2 2 0 1 1-4 0v-.09a1.7 1.7 0 0 0-1.12-1.56 1.7 1.7 0 0 0-1.87.34l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.7 1.7 0 0 0 .34-1.87 1.7 1.7 0 0 0-1.56-1.03H3a2 2 0 1 1 0-4h.09a1.7 1.7 0 0 0 1.56-1.12 1.7 1.7 0 0 0-.34-1.87l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.7 1.7 0 0 0 1.87.34h.01A1.7 1.7 0 0 0 10 3.09V3a2 2 0 1 1 4 0v.09a1.7 1.7 0 0 0 1.03 1.56 1.7 1.7 0 0 0 1.87-.34l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.7 1.7 0 0 0-.34 1.87v.01A1.7 1.7 0 0 0 20.91 10H21a2 2 0 1 1 0 4h-.09a1.7 1.7 0 0 0-1.51 1Z" />
        </svg>
      </button>

      <button
        className={`profile-chip ${hasSub ? "pro" : ""}`}
        title={account?.email}
        onClick={onOpenProfile}
      >
        <span className="profile-chip-avatar">{initial}</span>
        {hasSub && <span className="profile-chip-badge">PRO</span>}
      </button>
    </header>
  );
}
