import { KeyRound, MonitorUp, RotateCw } from "lucide-react";
import { useMemo, useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { HelpPopover } from "../../../lib/design/components/popover/help-popover";
import { Switch } from "../../../lib/design/components/switch/switch";
import { fillGuide } from "../../../lib/guides/guide-types";
import { useGuideRunner } from "../../../lib/guides/use-guide-runner";
import { useWorkspaces } from "../../../lib/hooks/use-workspaces";
import { useNavigate, useParams } from "../../../lib/router/router";
import { TerminalBus } from "../../../lib/terminal/terminal-bus";
import { terminalGuides } from "../../clyffy/guides/all-guides";
import { ClyffyPanel } from "../../clyffy/ui/clyffy-panel";
import { AppCards } from "../../apps/ui/app-cards";
import { TerminalPane } from "./terminal-pane";
import { WezTermDialog } from "./wezterm-dialog";

/** Workspace → Terminal: the workspace's Zellij session "shippin" in the browser (plain shell on
 * request), "Open in WezTerm" for a native terminal, and Clyffy's guides beside it. */
export function TerminalPage() {
  const { tenantId = "", workspaceId = "" } = useParams();
  const navigate = useNavigate();
  const [session, setSession] = useState(0);
  const [zellij, setZellij] = useState(true);
  const [wez, setWez] = useState(false);
  const ws = useWorkspaces(tenantId).data?.items.find((w) => w.id === workspaceId);
  const bus = useMemo(() => new TerminalBus(), []);
  const wsBase = `/t/${tenantId}/workspaces/${workspaceId}`;
  const guides = useMemo(
    () => terminalGuides.map((g) => fillGuide(g, { connectUrl: `${wsBase}/connect`, projectsUrl: `/t/${tenantId}/projects` })),
    [wsBase, tenantId],
  );
  const runner = useGuideRunner(guides, bus);
  return (
    <div className="-m-6 flex h-[calc(100vh-56px)] min-h-0 flex-col">
      <div className="flex items-center gap-2 border-b border-line px-4 py-2">
        <h1 className="text-sm font-semibold text-fg">Terminal</h1>
        <HelpPopover title="Zellij basics" placement="bottom">
          <p>This terminal opens the workspace's <strong className="text-fg">Zellij</strong> session <span className="font-mono">shippin</span>. Closing the tab keeps it running; come back and you're where you left off.</p>
          <p><span className="font-mono text-fg">Ctrl p</span> then <span className="font-mono text-fg">n</span>: new pane · <span className="font-mono text-fg">Ctrl t</span> then <span className="font-mono text-fg">n</span>: new tab · <span className="font-mono text-fg">Ctrl o</span> then <span className="font-mono text-fg">d</span>: detach. The bar at the bottom shows the keys.</p>
          <p>Turn Zellij off for a plain shell.</p>
        </HelpPopover>
        <span className="flex-1" />
        <Switch tooltip="On: the persistent Zellij session. Off: a plain shell" isSelected={zellij} onChange={(v) => { setZellij(v); setSession((n) => n + 1); }}>Zellij</Switch>
        <Button size="sm" variant="ghost" tooltip="Sign GitHub and the AI CLIs in" onPress={() => navigate(`${wsBase}/connect`)}><KeyRound className="size-4" /> Connect accounts</Button>
        <Button size="sm" variant="secondary" tooltip="Use your own WezTerm: download a config with an ssh domain to this workspace" onPress={() => setWez(true)}><MonitorUp className="size-4" /> Open in WezTerm</Button>
        <Button size="sm" variant="ghost" onPress={() => setSession((n) => n + 1)} tooltip={zellij ? "Reconnect to the Zellij session" : "Open a fresh shell"}><RotateCw className="size-4" /> {zellij ? "Reconnect" : "New session"}</Button>
      </div>
      <details open className="max-h-72 overflow-auto border-b border-line px-4 py-2">
        <summary className="cursor-pointer text-sm font-medium text-fg">Install an app</summary>
        <p className="mt-1 text-xs text-fg-muted">Copy a command and paste it into this terminal. It installs the app and starts the vendor sign-in, on your account.</p>
        <div className="mt-2"><AppCards tenantId={tenantId} /></div>
      </details>
      <div className="grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)_340px] overflow-hidden">
        <TerminalPane key={session} tenantId={tenantId} workspaceId={workspaceId} bus={bus} mux={zellij ? "zellij" : "plain"} />
        <ClyffyPanel runner={runner} onLink={navigate} />
      </div>
      <WezTermDialog isOpen={wez} onOpenChange={setWez} workspace={ws?.name ?? workspaceId.slice(0, 8)} connect={ws?.connect} />
    </div>
  );
}
