// Browser location store for the in-house router (replaces react-router: ~28 KB gz off the
// initial bundle, CONSOLE-IA budgets). Same-origin navigation uses the History API.
export type Loc = { pathname: string; search: string; hash: string };

const read = (): Loc => ({ pathname: window.location.pathname, search: window.location.search, hash: window.location.hash });
let current = read();
const subs = new Set<() => void>();
const emit = () => {
  current = read();
  subs.forEach((f) => f());
};
window.addEventListener("popstate", emit);

export const subscribe = (f: () => void) => {
  subs.add(f);
  return () => void subs.delete(f);
};
export const snapshot = () => current;

/** Go to `to` (absolute or relative to the current URL). Another origin is a full page load. */
export function navigate(to: string, opts?: { replace?: boolean }) {
  const u = new URL(to, window.location.href);
  if (u.origin !== window.location.origin) {
    window.location.assign(u.href);
    return;
  }
  const next = u.pathname + u.search + u.hash;
  if (next === window.location.pathname + window.location.search + window.location.hash) return;
  window.history[opts?.replace ? "replaceState" : "pushState"](null, "", next);
  emit();
  if (!u.hash) window.scrollTo(0, 0);
}
