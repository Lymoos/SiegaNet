import type { ActivationResult, ControlApi, Ok, Server, Status } from "./types";

/**
 * HTTP client for the real core control API on 127.0.0.1.
 *
 * NOTE: the core does not serve this API yet — this client is the (only)
 * integration point for the later "connect to the real core" step. The token
 * is expected to be issued by the core locally (e.g. written to a runtime
 * file readable by the same user); the UI keeps it in memory only.
 */
export class HttpControlApi implements ControlApi {
  constructor(
    private readonly port: number,
    private readonly token: string,
  ) {}

  private get base(): string {
    return `http://127.0.0.1:${this.port}`;
  }

  private async request<T>(path: string, init?: RequestInit): Promise<T> {
    const res = await fetch(`${this.base}${path}`, {
      ...init,
      headers: {
        Authorization: `Bearer ${this.token}`,
        "Content-Type": "application/json",
        ...init?.headers,
      },
    });
    if (!res.ok) {
      throw new Error(`control API ${path}: HTTP ${res.status}`);
    }
    return (await res.json()) as T;
  }

  getStatus(): Promise<Status> {
    return this.request<Status>("/status");
  }

  getServers(): Promise<Server[]> {
    return this.request<Server[]>("/servers");
  }

  connect(serverId: string): Promise<Ok> {
    return this.request<Ok>("/connect", {
      method: "POST",
      body: JSON.stringify({ server_id: serverId }),
    });
  }

  disconnect(): Promise<Ok> {
    return this.request<Ok>("/disconnect", { method: "POST" });
  }

  activate(key: string): Promise<ActivationResult> {
    return this.request<ActivationResult>("/activate", {
      method: "POST",
      body: JSON.stringify({ key }),
    });
  }
}
