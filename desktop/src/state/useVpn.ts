import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { controlApi } from "../api";
import type { Server, Status } from "../api/types";
import {
  loadLastConnection,
  loadSettings,
  saveLastConnection,
  type LastConnection,
} from "./settings";
import { onTrayCommand, updateTray } from "../tauri";

const STATUS_POLL_MS = 1000;
const SERVERS_POLL_MS = 8000;

const INITIAL_STATUS: Status = {
  state: "disconnected",
  server_id: null,
  inner_ip: null,
  since_unix: null,
  up_bytes: 0,
  down_bytes: 0,
};

export interface VpnStore {
  status: Status;
  servers: Server[];
  /** bytes/sec derived from consecutive /status polls */
  upRate: number;
  downRate: number;
  selectedId: string | null;
  last: LastConnection | null;
  /** server the big connect button will target */
  target: Server | null;
  connectedServer: Server | null;
  connect: (serverId: string) => void;
  disconnect: () => void;
  select: (serverId: string | null) => void;
}

/**
 * The single source of UI truth: polls the control API (mock or real core —
 * the hook cannot tell) and exposes status, servers and the connect/disconnect
 * actions. All VPN work happens on the other side of the API.
 */
export function useVpn(): VpnStore {
  const [status, setStatus] = useState<Status>(INITIAL_STATUS);
  const [servers, setServers] = useState<Server[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [last, setLast] = useState<LastConnection | null>(loadLastConnection);
  const [rates, setRates] = useState({ up: 0, down: 0 });

  const prevPoll = useRef<{ status: Status; at: number } | null>(null);
  const prevState = useRef(INITIAL_STATUS.state);
  // lets connect/disconnect refresh the status immediately instead of
  // waiting up to STATUS_POLL_MS for the next tick
  const pollNow = useRef<() => void>(() => {});

  // ---- status polling -----------------------------------------------------
  useEffect(() => {
    let alive = true;

    const poll = async () => {
      try {
        const st = await controlApi().getStatus();
        if (!alive) return;

        const now = Date.now();
        const prev = prevPoll.current;
        if (prev && st.state === "connected" && prev.status.state === "connected") {
          const dt = (now - prev.at) / 1000;
          if (dt > 0) {
            setRates({
              up: Math.max(0, (st.up_bytes - prev.status.up_bytes) / dt),
              down: Math.max(0, (st.down_bytes - prev.status.down_bytes) / dt),
            });
          }
        } else {
          setRates({ up: 0, down: 0 });
        }
        prevPoll.current = { status: st, at: now };

        if (st.state === "connected" && prevState.current !== "connected" && st.server_id) {
          setLast(saveLastConnection(st.server_id));
        }
        prevState.current = st.state;
        setStatus(st);
      } catch {
        // core unreachable (real mode, core not started yet) — show disconnected
        if (alive) setStatus(INITIAL_STATUS);
      }
    };

    pollNow.current = () => void poll();
    poll();
    const timer = setInterval(poll, STATUS_POLL_MS);
    return () => {
      alive = false;
      clearInterval(timer);
    };
  }, []);

  // ---- servers polling ----------------------------------------------------
  useEffect(() => {
    let alive = true;
    const poll = async () => {
      try {
        const list = await controlApi().getServers();
        if (alive) setServers(list);
      } catch {
        /* keep the previous list */
      }
    };
    poll();
    const timer = setInterval(poll, SERVERS_POLL_MS);
    return () => {
      alive = false;
      clearInterval(timer);
    };
  }, []);

  // ---- actions ------------------------------------------------------------
  const connect = useCallback((serverId: string) => {
    void controlApi()
      .connect(serverId)
      .then(() => pollNow.current());
  }, []);

  const disconnect = useCallback(() => {
    void controlApi()
      .disconnect()
      .then(() => pollNow.current());
  }, []);

  const select = useCallback((serverId: string | null) => {
    setSelectedId(serverId);
  }, []);

  // ---- derived ------------------------------------------------------------
  const connectedServer = useMemo(
    () => servers.find((s) => s.id === status.server_id) ?? null,
    [servers, status.server_id],
  );

  const target = useMemo(() => {
    const byId = (id: string | null | undefined) =>
      servers.find((s) => s.id === id) ?? null;
    return (
      byId(selectedId) ??
      connectedServer ??
      byId(last?.serverId) ??
      // fallback: best ping
      [...servers].sort((a, b) => a.ping_ms - b.ping_ms)[0] ??
      null
    );
  }, [servers, selectedId, connectedServer, last]);

  // ---- tray integration ---------------------------------------------------
  const trayCtx = useRef({ status, target, connect, disconnect });
  trayCtx.current = { status, target, connect, disconnect };

  useEffect(() => {
    const detail =
      status.state === "disconnected" || !connectedServer
        ? ""
        : `${connectedServer.city}, ${connectedServer.country}`;
    void updateTray(status.state, detail);
  }, [status.state, connectedServer]);

  useEffect(() => {
    let unlisten: (() => void) | undefined;
    void onTrayCommand((cmd) => {
      const { status: st, target: tg, connect: doConnect, disconnect: doDisconnect } =
        trayCtx.current;
      const wantDisconnect =
        cmd === "disconnect" || (cmd === "toggle" && st.state !== "disconnected");
      if (wantDisconnect) doDisconnect();
      else if (tg) doConnect(tg.id);
    }).then((fn) => {
      unlisten = fn;
    });
    return () => unlisten?.();
  }, []);

  // ---- auto-connect on launch ----------------------------------------------
  const autoTried = useRef(false);
  useEffect(() => {
    if (autoTried.current || servers.length === 0) return;
    autoTried.current = true;
    const s = loadSettings();
    if (s.autoConnect && last && servers.some((x) => x.id === last.serverId)) {
      connect(last.serverId);
    }
  }, [servers, last, connect]);

  return {
    status,
    servers,
    upRate: rates.up,
    downRate: rates.down,
    selectedId,
    last,
    target,
    connectedServer,
    connect,
    disconnect,
    select,
  };
}
