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
  | "play";

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
