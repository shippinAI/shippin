import { Download } from "lucide-react";
import { Button } from "../../../lib/design/components/button/button";
import { CodeBlock } from "../../../lib/design/components/code-block/code-block";
import { ModalDialog } from "../../../lib/design/components/dialog/modal-dialog";
import type { Connect } from "../../../lib/api/types";
import { domainName, weztermConfig } from "../../../lib/terminal/wezterm-config";

type Props = { isOpen: boolean; onOpenChange: (o: boolean) => void; workspace: string; connect?: Connect };

function download(name: string, text: string) {
  const url = URL.createObjectURL(new Blob([text], { type: "text/x-lua" }));
  const a = Object.assign(document.createElement("a"), { href: url, download: name });
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

/** "Open in WezTerm": the generated wezterm.lua (ssh mux domain → this workspace) and how to use it. */
export function WezTermDialog({ isOpen, onOpenChange, workspace, connect }: Props) {
  const name = domainName(workspace);
  const cfg = connect ? weztermConfig({ workspace, host: connect.host, port: connect.port, user: connect.user }) : "";
  return (
    <ModalDialog isOpen={isOpen} onOpenChange={onOpenChange} title="Open in WezTerm"
      description="Use your own WezTerm with this workspace. The connection uses your SSH key and joins the same Zellij session as this page.">
      <div className="space-y-4 px-6 py-5 text-sm text-fg-muted">
        {!connect ? (
          <p role="alert" className="text-warning">This workspace has no SSH address yet. Start it, then try again.</p>
        ) : (
          <>
            <ol className="list-decimal space-y-1 pl-5">
              <li>Download the config and save it as <span className="font-mono text-fg">~/.wezterm.lua</span> (or merge its <span className="font-mono">ssh_domains</span>).</li>
              <li>Add your SSH key under Account → SSH keys if you haven't.</li>
              <li>Run the command below, or pick <span className="font-mono">{name}</span> from WezTerm's launcher.</li>
            </ol>
            <CodeBlock label="Connect" code={`wezterm connect ${name}`} />
            <div className="flex flex-wrap gap-2">
              <Button variant="primary" tooltip="Download wezterm.lua for this workspace" onPress={() => download("wezterm.lua", cfg)}>
                <Download className="size-4" /> Download wezterm.lua
              </Button>
            </div>
          </>
        )}
      </div>
    </ModalDialog>
  );
}
