import type { ButtonSize, ButtonVariant } from "./button.types";

const base =
  "focus-ring inline-flex select-none items-center justify-center gap-2 whitespace-nowrap font-medium transition-[background,box-shadow,color,transform] duration-150 data-pressed:scale-[0.98] data-disabled:cursor-not-allowed data-disabled:opacity-45 cursor-default";

const variants: Record<ButtonVariant, string> = {
  primary:
    "bg-gradient-to-b from-primary to-primary-strong text-primary-fg shadow-[var(--shadow-e2)] data-hovered:shadow-[var(--shadow-glow)]",
  secondary:
    "bg-raised text-fg border border-line shadow-[var(--shadow-e1)] data-hovered:bg-overlay data-hovered:border-line-strong",
  ghost: "text-fg-muted data-hovered:bg-raised data-hovered:text-fg",
  danger: "bg-danger/15 text-danger border border-danger/30 data-hovered:bg-danger/25",
};

const sizes: Record<ButtonSize, string> = {
  sm: "h-8 rounded-[var(--radius-control)] px-3 text-[13px]",
  md: "h-9 rounded-[var(--radius-control)] px-3.5 text-sm",
  lg: "h-11 rounded-[0.625rem] px-5 text-[15px]",
};

export function buttonClass(variant: ButtonVariant = "secondary", size: ButtonSize = "md", extra = ""): string {
  return [base, variants[variant], sizes[size], extra].join(" ");
}
