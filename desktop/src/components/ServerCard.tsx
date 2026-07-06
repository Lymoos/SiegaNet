import type { RefObject } from "react";
import type { Server, Status } from "../api/types";
import { flagFor } from "../lib/flags";

const CARD_W = 260;
const CARD_H = 240;

interface Props {
  server: Server;
  status: Status;
  /** click position in container px; null => docked to the top-right corner */
  anchor: { x: number; y: number } | null;
  containerRef: RefObject<HTMLDivElement>;
  onConnect: (serverId: string) => void;
  onDisconnect: () => void;
  onClose: () => void;
}

export function ServerCard({
  server,
  status,
  anchor,
  containerRef,
  onConnect,
  onDisconnect,
  onClose,
}: Props) {
  const isActive = status.server_id === server.id && status.state !== "disconnected";
  const isConnected = isActive && status.state === "connected";
  const isConnecting = isActive && status.state === "connecting";

  let style: React.CSSProperties;
  if (anchor && containerRef.current) {
    const { clientWidth, clientHeight } = containerRef.current;
    const left = Math.min(Math.max(8, anchor.x + 14), clientWidth - CARD_W - 8);
    const top = Math.min(Math.max(8, anchor.y - 20), clientHeight - CARD_H - 8);
    style = { left, top };
  } else {
    style = { right: 20, top: 18 };
  }

  return (
    /* clicks inside the card must not reach the map's outside-click handler */
    <div className="server-card" style={style} onClick={(e) => e.stopPropagation()}>
      <button className="close" onClick={onClose} title="Закрыть">
        <svg width="12" height="12" viewBox="0 0 12 12" stroke="currentColor" strokeWidth="1.6">
          <path d="M1 1l10 10M11 1L1 11" />
        </svg>
      </button>

      <div className="card-head">
        <span className="card-flag">{flagFor(server.country)}</span>
        <div>
          <div className="card-country">{server.country}</div>
          <div className="card-city">{server.city}</div>
        </div>
      </div>

      <div className="card-row">
        <span>Пинг</span>
        <b>{server.ping_ms} ms</b>
      </div>
      <div className="card-row">
        <span>Нагрузка</span>
        <span style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <span className="load-track">
            <span
              className={`load-fill ${server.load_pct > 75 ? "hot" : ""}`}
              style={{ width: `${server.load_pct}%`, display: "block" }}
            />
          </span>
          <b>{server.load_pct}%</b>
        </span>
      </div>
      <div className="card-row">
        <span>Узел</span>
        <b>{server.host}</b>
      </div>

      <button
        className={`card-connect ${isConnected ? "off" : ""}`}
        onClick={() => (isActive ? onDisconnect() : onConnect(server.id))}
      >
        {isConnected ? "Отключиться" : isConnecting ? "Отмена" : "Подключиться"}
      </button>
    </div>
  );
}
