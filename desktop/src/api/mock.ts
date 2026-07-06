import type { ActivationResult, ControlApi, Ok, Server, Status, VpnState } from "./types";

/**
 * Fully self-contained mock of the core control API.
 *
 * Behaves like a live core: connect goes through a `connecting` phase before
 * `connected`, traffic counters accumulate while connected, pings jitter and
 * server load drifts. The UI must not be able to tell it apart from the real
 * API surface — same types, same semantics, async everywhere.
 */

interface MockServerSeed {
  id: string;
  country: string;
  city: string;
  host: string;
  basePing: number;
  baseLoad: number;
}

const SEEDS: MockServerSeed[] = [
  { id: "nl-ams-1", country: "Нидерланды", city: "Амстердам", host: "ams1.siega.net", basePing: 46, baseLoad: 34 },
  { id: "de-fra-1", country: "Германия", city: "Франкфурт", host: "fra1.siega.net", basePing: 51, baseLoad: 58 },
  { id: "fi-hel-1", country: "Финляндия", city: "Хельсинки", host: "hel1.siega.net", basePing: 28, baseLoad: 22 },
  { id: "se-sto-1", country: "Швеция", city: "Стокгольм", host: "sto1.siega.net", basePing: 37, baseLoad: 41 },
  { id: "gb-lon-1", country: "Великобритания", city: "Лондон", host: "lon1.siega.net", basePing: 62, baseLoad: 66 },
  { id: "fr-par-1", country: "Франция", city: "Париж", host: "par1.siega.net", basePing: 59, baseLoad: 48 },
  { id: "pl-waw-1", country: "Польша", city: "Варшава", host: "waw1.siega.net", basePing: 33, baseLoad: 29 },
  { id: "tr-ist-1", country: "Турция", city: "Стамбул", host: "ist1.siega.net", basePing: 74, baseLoad: 71 },
  { id: "kz-ala-1", country: "Казахстан", city: "Алматы", host: "ala1.siega.net", basePing: 88, baseLoad: 18 },
  { id: "ae-dxb-1", country: "ОАЭ", city: "Дубай", host: "dxb1.siega.net", basePing: 112, baseLoad: 52 },
  { id: "sg-sin-1", country: "Сингапур", city: "Сингапур", host: "sin1.siega.net", basePing: 168, baseLoad: 44 },
  { id: "jp-tyo-1", country: "Япония", city: "Токио", host: "tyo1.siega.net", basePing: 196, baseLoad: 37 },
  { id: "us-nyc-1", country: "США", city: "Нью-Йорк", host: "nyc1.siega.net", basePing: 128, baseLoad: 63 },
  { id: "us-lax-1", country: "США", city: "Лос-Анджелес", host: "lax1.siega.net", basePing: 182, baseLoad: 49 },
  { id: "br-sao-1", country: "Бразилия", city: "Сан-Паулу", host: "sao1.siega.net", basePing: 224, baseLoad: 26 },
];

function jitter(base: number, pct: number): number {
  return Math.max(1, Math.round(base * (1 + (Math.random() * 2 - 1) * pct)));
}

export class MockControlApi implements ControlApi {
  private state: VpnState = "disconnected";
  private serverId: string | null = null;
  private innerIp: string | null = null;
  private sinceUnix: number | null = null;

  private upBytes = 0;
  private downBytes = 0;
  /** last time traffic counters were advanced, ms */
  private lastTick = 0;
  /** current simulated rates, bytes/sec; random-walked on every tick */
  private upRate = 0;
  private downRate = 0;

  private connectTimer: ReturnType<typeof setTimeout> | null = null;
  private loads = new Map<string, number>(SEEDS.map((s) => [s.id, s.baseLoad]));

  async getStatus(): Promise<Status> {
    this.tickTraffic();
    return {
      state: this.state,
      server_id: this.serverId,
      inner_ip: this.innerIp,
      since_unix: this.sinceUnix,
      up_bytes: Math.round(this.upBytes),
      down_bytes: Math.round(this.downBytes),
    };
  }

  async getServers(): Promise<Server[]> {
    return SEEDS.map((s) => {
      // load drifts slowly within [5, 95]
      const prev = this.loads.get(s.id) ?? s.baseLoad;
      const next = Math.min(95, Math.max(5, prev + (Math.random() * 2 - 1) * 3));
      this.loads.set(s.id, next);
      return {
        id: s.id,
        country: s.country,
        city: s.city,
        host: s.host,
        ping_ms: jitter(s.basePing, 0.12),
        load_pct: Math.round(next),
      };
    });
  }

  async connect(serverId: string): Promise<Ok> {
    const seed = SEEDS.find((s) => s.id === serverId);
    if (!seed) return { ok: false };

    this.cancelPendingConnect();
    // switching servers while connected also passes through "connecting",
    // like a real re-handshake would
    this.state = "connecting";
    this.serverId = serverId;
    this.innerIp = null;
    this.sinceUnix = null;

    const handshakeMs = 1300 + Math.random() * 1200;
    this.connectTimer = setTimeout(() => {
      this.connectTimer = null;
      this.state = "connected";
      this.sinceUnix = Math.floor(Date.now() / 1000);
      this.innerIp = `10.77.0.${2 + (SEEDS.indexOf(seed) % 250)}`;
      this.upBytes = 0;
      this.downBytes = 0;
      this.upRate = 8_000 + Math.random() * 30_000;
      this.downRate = 60_000 + Math.random() * 400_000;
      this.lastTick = Date.now();
    }, handshakeMs);

    return { ok: true };
  }

  async activate(key: string): Promise<ActivationResult> {
    // mock: any non-empty key is a valid 30-day subscription; the real
    // check is done by the backend later, same request/response shape
    await new Promise((r) => setTimeout(r, 600 + Math.random() * 500));
    if (key.trim().length === 0) return { ok: false, valid_until: 0 };
    return {
      ok: true,
      valid_until: Math.floor(Date.now() / 1000) + 30 * 24 * 3600,
    };
  }

  async disconnect(): Promise<Ok> {
    this.cancelPendingConnect();
    this.state = "disconnected";
    this.serverId = null;
    this.innerIp = null;
    this.sinceUnix = null;
    this.upRate = 0;
    this.downRate = 0;
    return { ok: true };
  }

  private cancelPendingConnect() {
    if (this.connectTimer !== null) {
      clearTimeout(this.connectTimer);
      this.connectTimer = null;
    }
  }

  /** advance traffic counters lazily, on demand — no background interval */
  private tickTraffic() {
    if (this.state !== "connected") return;
    const now = Date.now();
    const dt = (now - this.lastTick) / 1000;
    this.lastTick = now;
    if (dt <= 0) return;

    // random-walk the rates with an occasional burst, so numbers look alive
    const burst = Math.random() < 0.06 ? 6 : 1;
    this.upRate = Math.max(1_000, this.upRate * (0.9 + Math.random() * 0.2));
    this.downRate = Math.max(8_000, this.downRate * (0.9 + Math.random() * 0.2));
    this.upBytes += this.upRate * dt * burst * 0.4;
    this.downBytes += this.downRate * dt * burst;
  }
}
