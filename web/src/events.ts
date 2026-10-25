export type EventKind =
  | "run.started"
  | "pr.blocker"
  | "pr.step"
  | "model.call"
  | "tool.call"
  | "verify.result"
  | "action"
  | "receipt"
  | "run.finished";

export interface UpkeepEvent {
  seq: number;
  kind: EventKind;
  time: string;
  run?: string;
  pr?: number;
  data?: unknown;
}

export type StreamStatus = "connecting" | "live" | "lost";

export interface StreamState {
  status: StreamStatus;
  session: string | null;
  version: string | null;
  events: UpkeepEvent[];
  gaps: number;
}

export type StreamAction =
  | { type: "hello"; session: string; version: string }
  | { type: "ping" }
  | { type: "event"; event: UpkeepEvent }
  | { type: "gap" }
  | { type: "lost" };

export const MAX_EVENTS = 50;

export const initialStream: StreamState = {
  status: "connecting",
  session: null,
  version: null,
  events: [],
  gaps: 0,
};

export function streamReducer(state: StreamState, action: StreamAction): StreamState {
  switch (action.type) {
    case "hello":
      return { ...state, status: "live", session: action.session, version: action.version };
    case "ping":
      return state.status === "live" ? state : { ...state, status: "live" };
    case "event":
      if (state.events.some((e) => e.seq === action.event.seq)) {
        return state;
      }
      return { ...state, events: [action.event, ...state.events].slice(0, MAX_EVENTS) };
    case "gap":
      return { ...state, events: [], gaps: state.gaps + 1 };
    case "lost":
      return { ...state, status: "lost" };
  }
}
