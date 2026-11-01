import type { ReactNode } from "react";
import { Dialog, Heading, Modal, ModalOverlay } from "react-aria-components";

type Props = {
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: ReactNode;
  children: ReactNode;
};

/** Centered modal (RAC) with a dimmed backdrop; Esc and outside click close it. */
export function ModalDialog({ isOpen, onOpenChange, title, description, children }: Props) {
  return (
    <ModalOverlay
      isOpen={isOpen}
      onOpenChange={onOpenChange}
      isDismissable
      className="fixed inset-0 z-50 flex items-center justify-center bg-canvas/70 p-6 backdrop-blur-[2px]"
    >
      <Modal className="w-full max-w-lg rounded-2xl border border-line-strong bg-surface shadow-[var(--shadow-e3)] outline-none">
        <Dialog className="outline-none">
          <div className="border-b border-line px-6 pt-5 pb-4">
            <Heading slot="title" className="text-[17px] font-semibold tracking-tight">{title}</Heading>
            {description && <p className="mt-1 text-[13px] text-fg-muted">{description}</p>}
          </div>
          {children}
        </Dialog>
      </Modal>
    </ModalOverlay>
  );
}
