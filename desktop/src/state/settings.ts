/**
 * UI settings, persisted in localStorage. Deliberately non-secret only:
 * path to the core binary, control port, UX toggles and the last connection.
 * The control token is NOT persisted — the UI receives it from the core at
 * runtime (late integration step) and keeps it in memory.
 */

export type ApiMode = "mock" | "core";

export interface Settings {
  apiMode: ApiMode;
  /** path to the sieganet-client binary that the app launches as a child */
  corePath: string;
  /** control-API port on 127.0.0.1 (must match the core config) */
  controlPort: number;
  /** in-memory only; never written to localStorage */
  controlToken: string;
  /** launch the app on OS login (tray) */
  autostart: boolean;
  /** connect to the last server right after app start */
  autoConnect: boolean;
}

export interface LastConnection {
  serverId: string;
  /** unix seconds of the last successful connect */
  ts: number;
}

const SETTINGS_KEY = "sieganet.settings.v1";
const LAST_KEY = "sieganet.last-connection.v1";

const DEFAULTS: Settings = {
  apiMode: "mock",
  corePath: "",
  controlPort: 7391,
  controlToken: "",
  autostart: false,
  autoConnect: false,
};

let tokenInMemory = "";

export function loadSettings(): Settings {
  try {
    const raw = localStorage.getItem(SETTINGS_KEY);
    if (!raw) return { ...DEFAULTS };
    const parsed = JSON.parse(raw) as Partial<Settings>;
    return { ...DEFAULTS, ...parsed, controlToken: tokenInMemory };
  } catch {
    return { ...DEFAULTS };
  }
}

export function saveSettings(s: Settings): void {
  tokenInMemory = s.controlToken;
  const { controlToken: _token, ...persistable } = s;
  localStorage.setItem(SETTINGS_KEY, JSON.stringify(persistable));
}

export function loadLastConnection(): LastConnection | null {
  try {
    const raw = localStorage.getItem(LAST_KEY);
    return raw ? (JSON.parse(raw) as LastConnection) : null;
  } catch {
    return null;
  }
}

export function saveLastConnection(serverId: string): LastConnection {
  const last: LastConnection = { serverId, ts: Math.floor(Date.now() / 1000) };
  localStorage.setItem(LAST_KEY, JSON.stringify(last));
  return last;
}
