import { useEffect, useRef, useState } from "react";
import type { BoardState } from "./board";
import { nextFocus } from "./focus";

const HOLD_MS = 1200;

export function useFocus(board: BoardState): number | null {
  const [current, setCurrent] = useState<number | null>(null);
  const boardRef = useRef(board);
  boardRef.current = board;
  const wanted = nextFocus(board, current);
  const idle = board.run === "idle";

  useEffect(() => {
    if (idle) {
      setCurrent(null);
    }
  }, [idle]);

  useEffect(() => {
    if (wanted === current) {
      return undefined;
    }
    const card = current !== null ? boardRef.current.cards[current] : undefined;
    const delay = card?.phase === "done" ? HOLD_MS : 0;
    const id = setTimeout(() => setCurrent(wanted), delay);
    return () => clearTimeout(id);
  }, [wanted, current]);

  return current ?? wanted;
}
