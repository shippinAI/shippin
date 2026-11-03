/** "Expires in 3d" from an ISO time (whole days; hours under a day). */
export function expiresIn(iso: string, now = Date.now()): string {
  const ms = new Date(iso).getTime() - now;
  if (ms <= 0) return "Expired";
  const h = Math.floor(ms / 3_600_000);
  return h < 24 ? `Expires in ${Math.max(h, 1)}h` : `Expires in ${Math.floor(h / 24)}d`;
}
