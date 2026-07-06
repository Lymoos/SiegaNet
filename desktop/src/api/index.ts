import { MockControlApi } from "./mock";
import { HttpControlApi } from "./http";
import type { ControlApi } from "./types";
import { loadSettings } from "../state/settings";

let instance: ControlApi | null = null;

/**
 * Single control-API client for the whole app.
 *
 * Mode comes from settings: "mock" (default, fully autonomous) or "core"
 * (real `sieganet-client` control API on 127.0.0.1). Switching mode in the
 * settings panel re-creates the client on next access.
 */
export function controlApi(): ControlApi {
  if (instance === null) {
    instance = createFromSettings();
  }
  return instance;
}

export function resetControlApi(): void {
  instance = null;
}

function createFromSettings(): ControlApi {
  const s = loadSettings();
  if (s.apiMode === "core") {
    // Token handoff from the core is part of the late integration step;
    // until then an empty token simply fails against a real core.
    return new HttpControlApi(s.controlPort, s.controlToken);
  }
  return new MockControlApi();
}
