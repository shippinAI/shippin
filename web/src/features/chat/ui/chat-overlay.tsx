import { chatUI, useChatUI } from "../../../lib/chat/chat-store";
import { Drawer } from "../../../lib/design/components/drawer/drawer";
import { ChatSurface } from "./chat-surface";

/** The global chat panel: a Drawer around the same ChatSurface as /chat. */
export function ChatOverlay() {
  const { open } = useChatUI();
  return (
    <Drawer isOpen={open} onOpenChange={chatUI.setOpen} title="Automaton" subtitle="Same conversations as the Chat page">
      <ChatSurface variant="overlay" />
    </Drawer>
  );
}
