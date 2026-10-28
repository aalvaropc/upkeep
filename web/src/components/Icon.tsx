export type IconName =
  | "mark"
  | "blocked"
  | "working"
  | "verified"
  | "done"
  | "connecting"
  | "live"
  | "lost"
  | "chip"
  | "play"
  | "again"
  | "close"
  | "person"
  | "bot"
  | "merge"
  | "patch"
  | "push"
  | "rules"
  | "wait"
  | "tool"
  | "learned"
  | "alert"
  | "search";

const PATHS: Record<IconName, string[]> = {
  mark: [
    "M5 6.5a2.5 2.5 0 1 0 5 0a2.5 2.5 0 1 0-5 0",
    "M5 17.5a2.5 2.5 0 1 0 5 0a2.5 2.5 0 1 0-5 0",
    "M14 12a2.5 2.5 0 1 0 5 0a2.5 2.5 0 1 0-5 0",
    "M7.5 9v6",
    "M10 6.5c5 0 6.5 2 6.5 3",
  ],
  blocked: ["M3 12a9 9 0 1 0 18 0a9 9 0 1 0-18 0", "M5.6 5.6l12.8 12.8"],
  working: ["M21 12a9 9 0 1 1-6.2-8.55", "M21 4v5h-5"],
  verified: ["M12 3l7 3v5c0 4.5-3 8.2-7 10-4-1.8-7-5.5-7-10V6z", "M9 12l2 2 4-4"],
  done: ["M3 12a9 9 0 1 0 18 0a9 9 0 1 0-18 0", "M8 12.5l2.7 2.7L16 9.5"],
  connecting: ["M5 12h.01", "M12 12h.01", "M19 12h.01"],
  live: [
    "M9 12a3 3 0 1 0 6 0a3 3 0 1 0-6 0",
    "M5.6 5.6a9 9 0 0 0 0 12.8",
    "M18.4 5.6a9 9 0 0 1 0 12.8",
  ],
  lost: ["M12 4l9 16H3z", "M12 10v4", "M12 17.5h.01"],
  chip: [
    "M7 7h10v10H7z",
    "M10 3v4",
    "M14 3v4",
    "M10 17v4",
    "M14 17v4",
    "M3 10h4",
    "M3 14h4",
    "M17 10h4",
    "M17 14h4",
  ],
  play: ["M7 5l12 7-12 7z"],
  again: ["M4 12a8 8 0 0 1 14-5.3", "M20 4v4h-4", "M20 12a8 8 0 0 1-14 5.3", "M4 20v-4h4"],
  close: ["M6 6l12 12", "M18 6L6 18"],
  person: ["M8 8a4 4 0 1 0 8 0a4 4 0 1 0-8 0", "M4 21c0-4 3.6-6 8-6s8 2 8 6"],
  bot: ["M5 9h14v10H5z", "M12 5v4", "M10 14h.01", "M14 14h.01", "M3 13h2", "M19 13h2"],
  merge: [
    "M6 4v10a4 4 0 0 0 4 4h8",
    "M6 4m-2 0a2 2 0 1 0 4 0a2 2 0 1 0-4 0",
    "M18 18m-2 0a2 2 0 1 0 4 0a2 2 0 1 0-4 0",
  ],
  patch: ["M7 3h8l4 4v14H7z", "M15 3v4h4", "M10 13h6", "M10 17h6"],
  push: ["M12 19V6", "M6 11l6-6 6 6", "M5 21h14"],
  rules: ["M12 3l8 3v6c0 4.5-3.2 7.7-8 9-4.8-1.3-8-4.5-8-9V6z", "M9 12h6", "M9 15h4"],
  wait: ["M3 12a9 9 0 1 0 18 0a9 9 0 1 0-18 0", "M12 7v5l3 2"],
  tool: ["M14.5 6.5a4 4 0 0 0-5 5L4 17l3 3 5.5-5.5a4 4 0 0 0 5-5L15 12l-3-3z"],
  alert: ["M12 3l9 16H3z", "M12 10v4", "M12 17h.01"],
  learned: ["M6 3h12v18l-6-4-6 4z", "M9 9.5l2 2 4-4"],
  search: [
    "M3 12a9 9 0 1 0 18 0a9 9 0 1 0-18 0",
    "M3.6 9h16.8",
    "M3.6 15h16.8",
    "M12 3c-2.5 2.6-3.6 5.6-3.6 9s1.1 6.4 3.6 9",
    "M12 3c2.5 2.6 3.6 5.6 3.6 9s-1.1 6.4-3.6 9",
  ],
};

interface Props {
  name: IconName;
  className?: string;
}

export function Icon({ name, className }: Props) {
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      width="1em"
      height="1em"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {PATHS[name].map((d) => (
        <path key={d} d={d} />
      ))}
    </svg>
  );
}
