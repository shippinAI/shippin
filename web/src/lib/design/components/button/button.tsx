import type { ReactNode } from "react";
import { Button as RacButton, type ButtonProps } from "react-aria-components";
import { Tooltip } from "../tooltip/tooltip";
import { buttonClass } from "./button-styles";
import type { ButtonSize, ButtonVariant } from "./button.types";

type Props = Omit<ButtonProps, "className" | "children"> & {
  variant?: ButtonVariant;
  size?: ButtonSize;
  tooltip: ReactNode;
  className?: string;
  children: ReactNode;
};

/** Text button. A tooltip is required: every action explains itself. */
export function Button({ variant, size, tooltip, className, children, ...rest }: Props) {
  return (
    <Tooltip content={tooltip}>
      <RacButton {...rest} className={buttonClass(variant, size, className)}>
        {children}
      </RacButton>
    </Tooltip>
  );
}
