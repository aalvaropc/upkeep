import { describe, expect, it } from "vitest";
import { MAX_EVENTS, initialStream, streamReducer, type UpkeepEvent } from "./events";

function event(seq: number): UpkeepEvent {
  return { seq, kind: "pr.step", time: "2026-10-25T09:00:00Z", pr: 5 };
}

describe("streamReducer", () => {
  it("goes live on hello and keeps the session and version", () => {
    const s = streamReducer(initialStream, { type: "hello", session: "s1", version: "v1.0.0" });
    expect(s.status).toBe("live");
    expect(s.session).toBe("s1");
    expect(s.version).toBe("v1.0.0");
  });

  it("goes back to live on a ping after the connection was lost", () => {
    const lost = streamReducer(initialStream, { type: "lost" });
    expect(lost.status).toBe("lost");
    expect(streamReducer(lost, { type: "ping" }).status).toBe("live");
  });

  it("keeps the newest events first, without duplicates, up to the limit", () => {
    let s = initialStream;
    for (let seq = 1; seq <= MAX_EVENTS + 5; seq++) {
      s = streamReducer(s, { type: "event", event: event(seq) });
    }
    s = streamReducer(s, { type: "event", event: event(MAX_EVENTS + 5) });
    expect(s.events).toHaveLength(MAX_EVENTS);
    expect(s.events[0]?.seq).toBe(MAX_EVENTS + 5);
  });

  it("clears the list on a gap because the server replays from its oldest event", () => {
    let s = streamReducer(initialStream, { type: "event", event: event(1) });
    s = streamReducer(s, { type: "gap" });
    expect(s.events).toHaveLength(0);
    expect(s.gaps).toBe(1);
  });
});
