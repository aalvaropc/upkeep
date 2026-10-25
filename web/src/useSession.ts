import { useCallback, useEffect, useReducer, useRef, useState } from "react";
import { api, ApiError } from "./api";
import { boardReducer, initialBoard, type BoardState, type RunStatus } from "./board";
import type { Detail, Info, Snapshot, UpkeepEvent } from "./types";

const STORAGE_KEY = "upkeep.session";

function remembered(): string | null {
  try {
    return window.sessionStorage.getItem(STORAGE_KEY);
  } catch {
    return null;
  }
}

function remember(id: string | null) {
  try {
    if (id) {
      window.sessionStorage.setItem(STORAGE_KEY, id);
    } else {
      window.sessionStorage.removeItem(STORAGE_KEY);
    }
  } catch {
    return;
  }
}

function explain(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 503) {
      return "The demo is full right now. Try again in a minute.";
    }
    if (err.status === 429) {
      return "Too many requests from here. Wait a minute and try again.";
    }
    return err.message;
  }
  return err instanceof Error ? err.message : "Something went wrong.";
}

export interface SessionApi {
  board: BoardState;
  info: Info | null;
  error: string | null;
  busy: boolean;
  sessionId: string | null;
  start: () => Promise<void>;
  again: () => Promise<void>;
  detail: (number: number) => Promise<Detail>;
}

export function useSession(): SessionApi {
  const [board, dispatch] = useReducer(boardReducer, initialBoard);
  const [info, setInfo] = useState<Info | null>(null);
  const [sessionId, setSessionId] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const creating = useRef(false);

  const adopt = useCallback((snap: Snapshot) => {
    remember(snap.id);
    setSessionId(snap.id);
    dispatch({
      type: "seed",
      prs: snap.prs,
      run: snap.status as RunStatus,
      position: snap.position,
    });
  }, []);

  const create = useCallback(async () => {
    if (creating.current) {
      return;
    }
    creating.current = true;
    try {
      adopt(await api.createSession());
      setError(null);
    } catch (err) {
      setError(explain(err));
    } finally {
      creating.current = false;
    }
  }, [adopt]);

  useEffect(() => {
    let cancelled = false;
    api
      .info()
      .then((i) => {
        if (!cancelled) {
          setInfo(i);
        }
      })
      .catch(() => undefined);
    const saved = remembered();
    const open = saved
      ? api.getSession(saved).then(
          (snap) => {
            if (!cancelled) {
              adopt(snap);
            }
          },
          () => (cancelled ? undefined : create()),
        )
      : create();
    void open;
    return () => {
      cancelled = true;
    };
  }, [adopt, create]);

  useEffect(() => {
    if (!sessionId) {
      return;
    }
    const source = new EventSource(`/api/sessions/${encodeURIComponent(sessionId)}/events`);
    source.addEventListener("hello", (e) => {
      const hello = JSON.parse((e as MessageEvent<string>).data) as {
        session: string;
        version: string;
      };
      dispatch({ type: "hello", session: hello.session, version: hello.version });
    });
    source.addEventListener("ping", () => dispatch({ type: "ping" }));
    source.addEventListener("gap", () => dispatch({ type: "gap" }));
    source.onmessage = (e) => dispatch({ type: "event", event: JSON.parse(e.data) as UpkeepEvent });
    source.onerror = () => dispatch({ type: "lost" });
    return () => source.close();
  }, [sessionId]);

  const again = useCallback(async () => {
    setBusy(true);
    try {
      const previous = sessionId;
      dispatch({ type: "reset" });
      setSessionId(null);
      remember(null);
      if (previous) {
        await api.deleteSession(previous).catch(() => undefined);
      }
      await create();
    } finally {
      setBusy(false);
    }
  }, [create, sessionId]);

  const start = useCallback(async () => {
    if (!sessionId) {
      await create();
      return;
    }
    if (board.run === "finished" || board.closed) {
      await again();
      return;
    }
    setBusy(true);
    try {
      await api.startRun(sessionId);
      setError(null);
    } catch (err) {
      setError(explain(err));
    } finally {
      setBusy(false);
    }
  }, [again, board.closed, board.run, create, sessionId]);

  const detail = useCallback(
    (number: number) => {
      if (!sessionId) {
        return Promise.reject(new Error("there is no session yet"));
      }
      return api.getPull(sessionId, number);
    },
    [sessionId],
  );

  return { board, info, error, busy, sessionId, start, again, detail };
}
