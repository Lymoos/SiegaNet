import { useMemo, useState } from "react";
import type { Server, Status } from "../api/types";
import type { LastConnection } from "../state/settings";
import { loadColor } from "../lib/flags";
import { formatAgo } from "../lib/format";
import { CountryBadge } from "./CountryBadge";

function pingClass(ms: number): string {
  if (ms < 80) return "ping-good";
  if (ms < 160) return "ping-mid";
  return "ping-far";
}

interface Props {
  servers: Server[];
  status: Status;
  selectedId: string | null;
  last: LastConnection | null;
  onSelect: (serverId: string) => void;
  onConnect: (serverId: string) => void;
}

export function Sidebar({ servers, status, selectedId, last, onSelect, onConnect }: Props) {
  const [query, setQuery] = useState("");

  const lastServer = useMemo(
    () => servers.find((s) => s.id === last?.serverId) ?? null,
    [servers, last],
  );

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    const list = q
      ? servers.filter(
          (s) =>
            s.country.toLowerCase().includes(q) || s.city.toLowerCase().includes(q),
        )
      : servers;
    return [...list].sort((a, b) => a.ping_ms - b.ping_ms);
  }, [servers, query]);

  const connectedToLast =
    lastServer !== null &&
    status.server_id === lastServer.id &&
    status.state !== "disconnected";

  return (
    <aside className="sidebar">
      <div className="brand">
        <div className="brand-glyph">
          {/* shield glyph */}
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <path d="M12 2 4 5.5v5.7c0 5 3.4 8.6 8 10.3 4.6-1.7 8-5.3 8-10.3V5.5L12 2Z" />
          </svg>
        </div>
        <div>
          <div className="brand-name">SiegaNet</div>
          <div className="brand-sub">stealth vpn</div>
        </div>
      </div>

      <div className="last-card">
        <div className="last-label">Последнее подключение</div>
        {lastServer ? (
          <>
            <div className="last-server">
              <CountryBadge country={lastServer.country} size={38} ring={loadColor(lastServer.load_pct)} />
              <div>
                <div className="last-country">{lastServer.country}</div>
                <div className="last-meta">
                  {lastServer.city}
                  {last ? ` · ${formatAgo(last.ts)}` : ""}
                </div>
              </div>
            </div>
            <button
              className="reconnect-btn"
              disabled={connectedToLast || status.state === "connecting"}
              onClick={() => onConnect(lastServer.id)}
            >
              {connectedToLast ? "Подключено" : "Переподключиться"}
            </button>
          </>
        ) : (
          <div className="last-empty">
            Ещё не подключались. Выберите сервер из списка или точку на карте.
          </div>
        )}
      </div>

      <div className="servers-head">
        <span className="servers-title">Серверы</span>
        <span className="servers-count">{servers.length}</span>
      </div>

      <div className="servers-search">
        <input
          placeholder="Поиск: страна или город"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </div>

      <div className="server-list">
        {filtered.map((s) => {
          const active = status.server_id === s.id && status.state !== "disconnected";
          return (
            <button
              key={s.id}
              className={`server-row ${selectedId === s.id ? "selected" : ""} ${active ? "active" : ""}`}
              onClick={() => onSelect(s.id)}
              onDoubleClick={() => onConnect(s.id)}
              title="Клик — показать на карте, двойной клик — подключиться"
            >
              <CountryBadge country={s.country} size={28} ring={loadColor(s.load_pct)} />
              <span className="server-name">
                <span className="server-country">{s.country}</span>
                <span className="server-city">{s.city}</span>
              </span>
              <span className={`server-ping ${pingClass(s.ping_ms)}`}>{s.ping_ms} ms</span>
            </button>
          );
        })}
      </div>
    </aside>
  );
}
