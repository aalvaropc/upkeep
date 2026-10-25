import type { Detail, Info, Snapshot } from "./types";

export class ApiError extends Error {
  readonly status: number;
  readonly retryAfter: number | null;

  constructor(status: number, message: string, retryAfter: number | null) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.retryAfter = retryAfter;
  }
}

async function request<T>(method: string, path: string): Promise<T> {
  const response = await fetch(path, { method, headers: { Accept: "application/json" } });
  if (response.status === 204) {
    return undefined as T;
  }
  let body: unknown = null;
  try {
    body = await response.json();
  } catch {
    body = null;
  }
  if (!response.ok) {
    const message =
      typeof body === "object" && body !== null && "error" in body && typeof body.error === "string"
        ? body.error
        : `the server answered ${response.status}`;
    const retry = Number(response.headers.get("Retry-After"));
    throw new ApiError(
      response.status,
      message,
      Number.isFinite(retry) && retry > 0 ? retry : null,
    );
  }
  return body as T;
}

const enc = encodeURIComponent;

export const api = {
  info: () => request<Info>("GET", "/api/info"),
  createSession: () => request<Snapshot>("POST", "/api/sessions"),
  getSession: (id: string) => request<Snapshot>("GET", `/api/sessions/${enc(id)}`),
  startRun: (id: string) => request<{ position: number }>("POST", `/api/sessions/${enc(id)}/run`),
  getPull: (id: string, number: number) =>
    request<Detail>("GET", `/api/sessions/${enc(id)}/prs/${number}`),
  deleteSession: (id: string) => request<void>("DELETE", `/api/sessions/${enc(id)}`),
};
