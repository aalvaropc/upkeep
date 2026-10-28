import { useEffect, useId, useRef, type KeyboardEvent, type ReactNode } from "react";
import styles from "./Tabs.module.css";

export interface Tab {
  id: string;
  label: string;
  content: ReactNode;
}

interface Props {
  label: string;
  tabs: Tab[];
  selected: string;
  onSelect: (id: string) => void;
}

export function Tabs({ label, tabs, selected, onSelect }: Props) {
  const base = useId();
  const buttons = useRef<(HTMLButtonElement | null)[]>([]);
  const current = Math.max(
    0,
    tabs.findIndex((t) => t.id === selected),
  );
  const shown = useRef(current);

  useEffect(() => {
    if (shown.current === current) {
      return;
    }
    shown.current = current;
    if (document.activeElement === null || document.activeElement === document.body) {
      buttons.current[current]?.focus();
    }
  }, [current]);

  const move = (index: number) => {
    const count = tabs.length;
    const next = tabs[((index % count) + count) % count];
    if (!next) {
      return;
    }
    onSelect(next.id);
    buttons.current[tabs.indexOf(next)]?.focus();
  };

  const onKeyDown = (e: KeyboardEvent<HTMLButtonElement>) => {
    switch (e.key) {
      case "ArrowRight":
        move(current + 1);
        break;
      case "ArrowLeft":
        move(current - 1);
        break;
      case "Home":
        move(0);
        break;
      case "End":
        move(tabs.length - 1);
        break;
      default:
        return;
    }
    e.preventDefault();
  };

  return (
    <div className={styles.tabs}>
      <div role="tablist" aria-label={label} className={styles.list}>
        {tabs.map((t, i) => {
          const active = i === current;
          return (
            <button
              key={t.id}
              ref={(el) => {
                buttons.current[i] = el;
              }}
              type="button"
              role="tab"
              id={`${base}-tab-${t.id}`}
              aria-selected={active}
              aria-controls={`${base}-panel-${t.id}`}
              tabIndex={active ? 0 : -1}
              className={styles.tab}
              onClick={() => onSelect(t.id)}
              onKeyDown={onKeyDown}
            >
              {t.label}
            </button>
          );
        })}
      </div>
      {tabs.map((t, i) => (
        <div
          key={t.id}
          role="tabpanel"
          id={`${base}-panel-${t.id}`}
          aria-labelledby={`${base}-tab-${t.id}`}
          hidden={i !== current}
          tabIndex={i === current ? 0 : -1}
          className={styles.panel}
        >
          {i === current ? t.content : null}
        </div>
      ))}
    </div>
  );
}
