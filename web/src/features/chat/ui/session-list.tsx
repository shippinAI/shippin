import { MessageSquare } from "lucide-react";
import { ListBox, ListBoxItem } from "react-aria-components";
import { useChatSessions } from "../../../lib/chat/use-chat";
import { timeAgo } from "../../../lib/format";

/** Your chats, newest activity first. */
export function SessionList({ selected, onSelect }: { selected: string | null; onSelect: (id: string) => void }) {
  const items = useChatSessions().data?.items ?? [];
  return (
    <ListBox aria-label="Chats" selectionMode="single" selectedKeys={selected ? [selected] : []}
      onAction={(k) => onSelect(String(k))} className="space-y-0.5 p-2" renderEmptyState={() => <p className="p-2 text-base text-fg-muted">No chats yet.</p>}>
      {items.map((s) => (
        <ListBoxItem key={s.id} id={s.id} textValue={s.title}
          className="flex cursor-default items-start gap-2 rounded-lg px-2 py-1.5 text-base outline-none data-focused:bg-raised data-selected:bg-primary/10">
          <MessageSquare className="mt-0.5 size-4 shrink-0 text-fg-muted" aria-hidden />
          <span className="min-w-0"><span className="block truncate text-fg">{s.title}</span><span className="text-xs text-fg-subtle">{timeAgo(s.updated_at)}</span></span>
        </ListBoxItem>
      ))}
    </ListBox>
  );
}
