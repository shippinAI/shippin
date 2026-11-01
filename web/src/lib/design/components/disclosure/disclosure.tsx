import { ChevronRight } from "lucide-react";
import type { ReactNode } from "react";
import { Button, Disclosure as RacDisclosure, DisclosurePanel, Heading } from "react-aria-components";

/** Collapsed-by-default section for technical details. */
export function Disclosure({ title, children }: { title: string; children: ReactNode }) {
  return (
    <RacDisclosure className="group rounded-[var(--radius-card)] border border-line bg-surface">
      <Heading>
        <Button slot="trigger" className="focus-ring flex w-full cursor-default items-center gap-2 px-5 py-3.5 text-left text-sm font-medium text-fg-muted data-hovered:text-fg">
          <ChevronRight className="size-4 transition-transform group-data-expanded:rotate-90" />
          {title}
        </Button>
      </Heading>
      <DisclosurePanel className="border-t border-line">{children}</DisclosurePanel>
    </RacDisclosure>
  );
}
