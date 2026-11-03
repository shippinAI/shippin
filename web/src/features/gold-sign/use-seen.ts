import { useEffect, useRef, useState } from "react";

/**
 * True once the returned ref's element has been on screen above the sticky decision bar
 * (bottom 120 px excluded). Used to keep Sign disabled until the commands were scrolled to the end.
 * Resets when `key` changes (another ticket or a new hash).
 */
export function useSeen<T extends Element>(key: string) {
  const ref = useRef<T>(null);
  const [seen, setSeen] = useState(false);
  useEffect(() => {
    setSeen(false);
    const el = ref.current;
    if (!el) return;
    const io = new IntersectionObserver((es) => {
      if (es.some((e) => e.isIntersecting)) {
        setSeen(true);
        io.disconnect();
      }
    }, { rootMargin: "0px 0px -120px 0px" });
    io.observe(el);
    return () => io.disconnect();
  }, [key]);
  return [ref, seen] as const;
}
