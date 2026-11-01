import { Bot, Square, TerminalSquare } from "lucide-react";
import { useState } from "react";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { toast } from "../../../lib/design/components/toast/toast-queue";
import { useAISessions, useLaunch, useStop } from "../use-ai-sessions";
import { AITerminal } from "./ai-terminal";

/** One click starts Codex CLI or Claude Code in a persistent session; reopening the page resumes it. */
export function LauncherPage() {
  const list = useAISessions();
  const launch = useLaunch();
  const stop = useStop();
  const [open, setOpen] = useState<string | null>(null);
  const sessions = list.data?.sessions ?? [];
  const active = open ?? sessions.find((s) => s.running)?.id ?? null;
  const start = (id: string) => launch.mutate(id, { onSuccess: () => setOpen(id), onError: (e) => toast.error("Could not start", e.message) });
  return (
    <main className="flex h-dvh flex-col gap-3 bg-canvas p-4 text-fg">
      <header className="flex flex-wrap items-center gap-2">
        <Bot className="size-5 text-primary" aria-hidden />
        <h1 className="mr-4 text-[17px] font-semibold tracking-tight">AI launcher</h1>
        {sessions.map((s) => (
          <div key={s.id} className="flex items-center gap-1 rounded-xl border border-line bg-raised/60 p-1 pl-2">
            <Badge tone={s.running ? "success" : "neutral"}>{s.running ? "running" : "stopped"}</Badge>
            <Button variant={active === s.id ? "primary" : "ghost"} isDisabled={launch.isPending}
              tooltip={s.running ? `Open ${s.label} (resumes where you left off)` : `Start ${s.label} in a persistent session`}
              onPress={() => start(s.id)}>
              <TerminalSquare className="size-4" aria-hidden /> {s.running ? `Open ${s.label}` : `Launch ${s.label}`}
            </Button>
            {s.running && <Button variant="ghost" tooltip={`End the ${s.label} session`} onPress={() => { stop.mutate(s.id); if (active === s.id) setOpen(null); }}>
              <Square className="size-3.5" aria-hidden /><span className="sr-only">Stop {s.label}</span></Button>}
          </div>
        ))}
        {list.isError && <span role="alert" className="text-danger">Launcher unavailable: {list.error.message}</span>}
      </header>
      <p className="text-[13px] text-fg-muted">
        Sign in inside the tool with your own subscription (its normal login). Closing this tab or losing the network only
        detaches: the session keeps running and reopens here.
      </p>
      <section className="min-h-0 flex-1" aria-label="Session">
        {active
          ? <AITerminal key={active} id={active} />
          : <div className="grid h-full place-items-center rounded-xl border border-dashed border-line text-fg-subtle">Pick a tool to start.</div>}
      </section>
    </main>
  );
}
