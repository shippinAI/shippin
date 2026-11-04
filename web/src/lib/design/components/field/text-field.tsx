import { FieldError, Input, Label, Text, TextArea, TextField as RacTextField, type InputProps, type TextFieldProps } from "react-aria-components";

type Props = Omit<TextFieldProps, "className"> & {
  label: string;
  description?: string;
  placeholder?: string;
  multiline?: boolean;
  mono?: boolean;
} & Pick<InputProps, "min" | "max" | "step" | "type">;

const control =
  "w-full rounded-[var(--radius-control)] border border-line bg-canvas/60 px-3 text-sm text-fg placeholder:text-fg-subtle outline-none transition-colors data-hovered:border-line-strong data-focused:border-primary data-focused:ring-2 data-focused:ring-primary/25 data-invalid:border-danger";

/** Labeled text input or textarea with RAC validation messages. */
export function TextField({ label, description, placeholder, multiline, mono, min, max, step, type = "text", ...rest }: Props) {
  return (
    <RacTextField {...rest} className="flex flex-col gap-1.5">
      <Label className="text-[13px] font-medium text-fg">{label}</Label>
      {multiline ? (
        <TextArea rows={4} placeholder={placeholder} className={`${control} resize-none py-2 ${mono ? "font-mono text-[12px]" : ""}`} />
      ) : (
        <Input type={type} min={min} max={max} step={step} placeholder={placeholder} className={`${control} h-9 ${mono ? "font-mono" : ""}`} />
      )}
      {description && <Text slot="description" className="text-xs text-fg-subtle">{description}</Text>}
      <FieldError className="text-xs text-danger" />
    </RacTextField>
  );
}
