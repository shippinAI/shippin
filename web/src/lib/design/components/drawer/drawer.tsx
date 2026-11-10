import { X } from "lucide-react";
import type { ReactNode } from "react";
import { Dialog, Heading, Modal, ModalOverlay } from "react-aria-components";
import { IconButton } from "../button/icon-button";

type Props = { isOpen: boolean; onOpenChange: (o: boolean) => void; title: ReactNode; subtitle?: ReactNode; children: ReactNode; footer?: ReactNode };

/** Right-side sheet (RAC Modal). Esc / outside click closes. */
export function Drawer({ isOpen, onOpenChange, title, subtitle, children, footer }: Props) {
  return (
    <ModalOverlay isOpen={isOpen} onOpenChange={onOpenChange} isDismissable className="fixed inset-0 z-50 bg-scrim">
      <Modal className="fixed inset-y-0 right-0 flex w-full max-w-130 flex-col border-l border-line-strong bg-surface shadow-e3 outline-none data-entering:animate-enter motion-reduce:animate-none">
        <Dialog className="flex h-full flex-col outline-none">
          <div className="flex items-start justify-between gap-3 border-b border-line px-6 py-4">
            <div className="min-w-0">
              <Heading slot="title" className="truncate text-lg font-semibold tracking-tight">{title}</Heading>
              {subtitle && <div className="mt-0.5 text-sm text-fg-muted">{subtitle}</div>}
            </div>
            <IconButton icon={X} label="Close panel" onPress={() => onOpenChange(false)} />
          </div>
          <div className="flex-1 overflow-y-auto">{children}</div>
          {footer && <div className="flex flex-wrap items-center gap-2 border-t border-line px-6 py-3.5">{footer}</div>}
        </Dialog>
      </Modal>
    </ModalOverlay>
  );
}
