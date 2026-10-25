import { useEffect, useRef, type ReactNode } from "react";
import { Icon } from "./Icon";
import styles from "./Overlay.module.css";

interface Props {
  title: string;
  subtitle?: string;
  side?: boolean;
  onClose: () => void;
  children: ReactNode;
}

const FOCUSABLE =
  'a[href], button:not([disabled]), textarea, input, select, [tabindex]:not([tabindex="-1"])';

export function Overlay({ title, subtitle, side = false, onClose, children }: Props) {
  const panel = useRef<HTMLDivElement>(null);
  const close = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    close.current?.focus();
    return () => {
      document.body.style.overflow = overflow;
      previous?.focus();
    };
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        onClose();
        return;
      }
      if (e.key !== "Tab" || !panel.current) {
        return;
      }
      const items = Array.from(panel.current.querySelectorAll<HTMLElement>(FOCUSABLE));
      const first = items[0];
      const last = items[items.length - 1];
      if (!first || !last) {
        return;
      }
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div
      className={styles.backdrop}
      data-side={side}
      onMouseDown={(e) => e.target === e.currentTarget && onClose()}
    >
      <div ref={panel} className={styles.panel} role="dialog" aria-modal="true" aria-label={title}>
        <header className={styles.head}>
          <div className={styles.titles}>
            <h2 className={styles.title}>{title}</h2>
            {subtitle ? <p className={styles.subtitle}>{subtitle}</p> : null}
          </div>
          <button
            ref={close}
            type="button"
            className={styles.close}
            onClick={onClose}
            aria-label="Close"
          >
            <Icon name="close" />
          </button>
        </header>
        <div className={styles.body}>{children}</div>
      </div>
    </div>
  );
}
