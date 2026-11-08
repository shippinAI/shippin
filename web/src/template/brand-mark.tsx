/** Shippin logo mark: stacked layers in a gradient tile. */
export function BrandMark({ size = 28, logoURL }: { size?: number; logoURL?: string }) {
  if (logoURL) return <img src={logoURL} width={size} height={size} className="rounded object-contain" alt="" />;
  return (
    <svg width={size} height={size} viewBox="0 0 32 32" aria-hidden>
      <defs>
        <linearGradient id="bm" x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="oklch(0.8 0.13 225)" />
          <stop offset="1" stopColor="oklch(0.6 0.2 285)" />
        </linearGradient>
      </defs>
      <rect width="32" height="32" rx="8" fill="url(#bm)" />
      <path d="M8 19l8 5 8-5M8 13l8-5 8 5-8 5z" fill="none" stroke="#fff" strokeWidth="2.2" strokeLinejoin="round" />
    </svg>
  );
}
