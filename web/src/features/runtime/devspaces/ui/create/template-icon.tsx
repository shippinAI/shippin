import { Atom, Box, Disc3, FileCode2, Zap, type LucideIcon } from "lucide-react";

// Icon key (stored on the template) → glyph and module hue. Unknown keys fall back to a box.
const icons: Record<string, [LucideIcon, string]> = {
  react: [Atom, "border-hue-data/30 bg-hue-data/15 text-hue-data"],
  python: [FileCode2, "border-hue-secrets/30 bg-hue-secrets/15 text-hue-secrets"],
  go: [Zap, "border-hue-network/30 bg-hue-network/15 text-hue-network"],
  debian: [Disc3, "border-hue-ai/30 bg-hue-ai/15 text-hue-ai"],
};
const fallback = "border-primary/30 bg-primary/15 text-primary";

export const iconKeys = Object.keys(icons);

export function TemplateIcon({ icon, size = 40 }: { icon: string; size?: number }) {
  const [Icon, tone] = icons[icon] ?? [Box, fallback];
  return (
    <span className={`inline-flex shrink-0 items-center justify-center rounded-xl border ${tone}`} style={{ width: size, height: size }}>
      <Icon style={{ width: size * 0.5, height: size * 0.5 }} strokeWidth={1.75} aria-hidden />
    </span>
  );
}
