import { lazy, Suspense } from "react";
import { MessageCircle } from "lucide-react";
import { Button } from "react-aria-components";
import { chatUI, useChatUI } from "../../../lib/chat/chat-store";
import { useLocation } from "../../../lib/router/router";

const ChatOverlay = lazy(() => import("./chat-overlay").then((m) => ({ default: m.ChatOverlay })));

/** Floating "Chat" button on every signed-in page; the panel itself loads on first open. */
export function ChatLauncher() {
  const { open } = useChatUI();
  const { pathname } = useLocation();
  if (pathname.startsWith("/chat") || pathname === "/login" || pathname === "/device") return null;
  return (
    <>
      <Button aria-label="Open chat" onPress={() => chatUI.openOverlay()}
        className="fixed bottom-5 right-5 z-40 flex size-12 items-center justify-center rounded-full bg-primary text-white shadow-[var(--shadow-e3)] outline-none data-focus-visible:ring-2 data-focus-visible:ring-primary/40">
        <MessageCircle className="size-5" aria-hidden />
      </Button>
      {open && <Suspense fallback={null}><ChatOverlay /></Suspense>}
    </>
  );
}
