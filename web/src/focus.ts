import type { BoardState, Card } from "./board";

function lastAt(card: Card): number {
  const last = card.trace[card.trace.length - 1];
  return Math.max(last?.at ?? 0, card.startedAt ?? 0);
}

export function liveCard(board: BoardState): Card | null {
  let best: Card | null = null;
  for (const n of board.order) {
    const card = board.cards[n];
    if (!card || (card.phase !== "working" && card.phase !== "verified")) {
      continue;
    }
    if (!best || lastAt(card) > lastAt(best)) {
      best = card;
    }
  }
  return best;
}

export function activeCount(board: BoardState): number {
  return board.order.filter((n) => {
    const phase = board.cards[n]?.phase;
    return phase === "working" || phase === "verified";
  }).length;
}

export function queueOrder(board: BoardState): number[] {
  const planned = (board.plan?.order ?? []).filter((n) => board.cards[n]);
  const rest = board.order.filter((n) => !planned.includes(n));
  return [...planned, ...rest];
}
