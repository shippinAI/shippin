import type { ReactNode } from "react";
import { Link } from "react-aria-components";
import { Tooltip } from "../tooltip/tooltip";
import { buttonClass } from "./button-styles";
import type { ButtonSize, ButtonVariant } from "./button.types";

type Props = {
  href: string;
  tooltip: ReactNode;
  variant?: ButtonVariant;
  size?: ButtonSize;
  className?: string;
  children: ReactNode;
};

/** Navigation styled as a button (full page or in-app). */
export function LinkButton({ href, tooltip, variant, size, className, children }: Props) {
  return (
    <Tooltip content={tooltip}>
      <Link href={href} className={buttonClass(variant, size, className)}>
        {children}
      </Link>
    </Tooltip>
  );
}
