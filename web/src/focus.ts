import type { BoardState, Card } from "./board";

function working(card: Card | undefined): card is Card {
  return card !== undefined && (card.phase === "working" || card.phase === "verified");
}

export function workingCards(board: BoardState): Card[] {
  return queueOrder(board)
    .map((n) => board.cards[n])
    .filter(working);
}

export function nextFocus(board: BoardState, current: number | null): number | null {
  if (current !== null && working(board.cards[current])) {
    return current;
  }
  let best: Card | null = null;
  for (const card of workingCards(board)) {
    if (!best || (card.startedAt ?? 0) < (best.startedAt ?? 0)) {
      best = card;
    }
  }
  return best ? best.pr.number : current;
}

export function queueOrder(board: BoardState): number[] {
  const planned = (board.plan?.order ?? []).filter((n) => board.cards[n]);
  const rest = board.order.filter((n) => !planned.includes(n));
  return [...planned, ...rest];
}
