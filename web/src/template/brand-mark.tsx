/** The installation's logo when one is configured, otherwise the default mark: stacked layers on a flat primary tile. */
export function BrandMark({ size = 28, logoURL }: { size?: number; logoURL?: string }) {
  if (logoURL) return <img src={logoURL} width={size} height={size} className="rounded-sm object-contain" alt="" />;
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" className="text-primary" aria-hidden>
      <rect width="32" height="32" rx="8" fill="currentColor" />
      <path d="M8 19l8 5 8-5M8 13l8-5 8 5-8 5z" fill="none" className="stroke-primary-fg" strokeWidth="2.2" strokeLinejoin="round" />
    </svg>
  );
}
