/** Reads a design token at runtime, for code that cannot use CSS, such as xterm's theme. Empty when there is no document. */
export function tokenValue(name: `--${string}`): string {
  if (typeof document === "undefined") return "";
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}
