import { useEffect, useReducer } from "react";
import { initialStream, streamReducer, type StreamState, type UpkeepEvent } from "./events";

interface Hello {
  session: string;
  version: string;
}

export function useEventStream(session: string): StreamState {
  const [state, dispatch] = useReducer(streamReducer, initialStream);

  useEffect(() => {
    const source = new EventSource(`/api/sessions/${encodeURIComponent(session)}/events`);
    source.addEventListener("hello", (e) => {
      const hello = JSON.parse(e.data) as Hello;
      dispatch({ type: "hello", session: hello.session, version: hello.version });
    });
    source.addEventListener("ping", () => dispatch({ type: "ping" }));
    source.addEventListener("gap", () => dispatch({ type: "gap" }));
    source.onmessage = (e) => dispatch({ type: "event", event: JSON.parse(e.data) as UpkeepEvent });
    source.onerror = () => dispatch({ type: "lost" });
    return () => source.close();
  }, [session]);

  return state;
}
