import { Atom, Box, Disc3, FileCode2, Zap, type LucideIcon } from "lucide-react";

// Icon key (stored on the template) → glyph + brand hue. Unknown keys fall back to a box.
const icons: Record<string, [LucideIcon, string]> = {
  react: [Atom, "#61dafb"], python: [FileCode2, "#4b8bbe"], go: [Zap, "#00add8"], debian: [Disc3, "#d70a53"],
};

export const iconKeys = Object.keys(icons);

export function TemplateIcon({ icon, size = 40 }: { icon: string; size?: number }) {
  const [Icon, hue] = icons[icon] ?? [Box, "var(--color-primary)"];
  return (
    <span
      className="inline-flex shrink-0 items-center justify-center rounded-xl border"
      style={{ width: size, height: size, color: hue, background: `color-mix(in oklch, ${hue} 14%, transparent)`, borderColor: `color-mix(in oklch, ${hue} 30%, transparent)` }}
    >
      <Icon style={{ width: size * 0.5, height: size * 0.5 }} strokeWidth={1.75} />
    </span>
  );
}
