/**
 * Control-API contract of the Go core (`sieganet-client`).
 *
 * The core exposes a local HTTP API on 127.0.0.1:<port> protected by a local
 * token. The UI is a shell over this contract and NEVER implements any VPN
 * logic itself. While the core does not serve this API yet, the UI runs on
 * MockControlApi (api/mock.ts) which implements the exact same interface.
 *
 *   GET  /status      -> Status
 *   GET  /servers     -> Server[]
 *   POST /connect     { server_id } -> { ok }
 *   POST /disconnect               -> { ok }
 *   POST /activate    { key } -> { ok, valid_until }
 *
 * Activation model (subscription VPN): without a valid activation key the
 * client refuses to connect. The key is checked via POST /activate (mocked
 * for now — real check goes to the backend later) and the result is cached
 * locally so the key is not asked for on every launch.
 */

export type VpnState = "disconnected" | "connecting" | "connected";

export interface Status {
  state: VpnState;
  /** id of the server we are connected/connecting to; null when disconnected */
  server_id: string | null;
  /** inner tunnel IP assigned by the server; null when disconnected */
  inner_ip: string | null;
  /** unix seconds when the current session was established; null otherwise */
  since_unix: number | null;
  up_bytes: number;
  down_bytes: number;
}

export interface Server {
  id: string;
  country: string;
  city: string;
  host: string;
  ping_ms: number;
  load_pct: number;
}

export interface Ok {
  ok: boolean;
}

export interface ActivationResult {
  ok: boolean;
  /** unix seconds until which the subscription is valid; 0 when !ok */
  valid_until: number;
}

export interface ControlApi {
  getStatus(): Promise<Status>;
  getServers(): Promise<Server[]>;
  connect(serverId: string): Promise<Ok>;
  disconnect(): Promise<Ok>;
  activate(key: string): Promise<ActivationResult>;
}
