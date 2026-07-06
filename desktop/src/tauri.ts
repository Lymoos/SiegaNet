/**
 * Thin, guarded bridge to the Tauri shell (tray + core process management).
 * Every call is a no-op in a plain browser, so `npm run dev` + a browser tab
 * exercises the full UI on the mock API without Tauri.
 */

import type { VpnState } from "./api/types";

export function isTauri(): boolean {
  return typeof window !== "undefined" && "__TAURI_INTERNALS__" in window;
}

/** Reflect the VPN state in the tray icon tooltip + menu. */
export async function updateTray(state: VpnState, detail: string): Promise<void> {
  if (!isTauri()) return;
  const { invoke } = await import("@tauri-apps/api/core");
  await invoke("set_tray_status", { state, detail }).catch(() => {});
}

export type TrayCommand = "toggle" | "connect" | "disconnect";

/** Tray menu clicks arrive as `tray-command` events emitted from Rust. */
export async function onTrayCommand(
  cb: (cmd: TrayCommand) => void,
): Promise<() => void> {
  if (!isTauri()) return () => {};
  const { listen } = await import("@tauri-apps/api/event");
  const unlisten = await listen<TrayCommand>("tray-command", (e) => cb(e.payload));
  return unlisten;
}

/**
 * Launch / stop the Go core (`sieganet-client`) as an elevated child process.
 * Elevation is required by the core (TUN device, routes, firewall rules) and
 * is handled per-OS on the Rust side.
 */
export async function startCore(path: string, args: string[]): Promise<string> {
  if (!isTauri()) throw new Error("недоступно вне Tauri");
  const { invoke } = await import("@tauri-apps/api/core");
  return invoke<string>("core_start", { path, args });
}

export async function stopCore(): Promise<string> {
  if (!isTauri()) throw new Error("недоступно вне Tauri");
  const { invoke } = await import("@tauri-apps/api/core");
  return invoke<string>("core_stop");
}

export async function setAutostart(enabled: boolean): Promise<void> {
  if (!isTauri()) return;
  const { enable, disable } = await import("@tauri-apps/plugin-autostart");
  if (enabled) await enable();
  else await disable();
}
