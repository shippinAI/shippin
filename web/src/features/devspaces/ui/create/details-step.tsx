import { GitBranch, KeyRound } from "lucide-react";
import { TextField } from "../../../../lib/design/components/field/text-field";
import type { Template } from "../../../../lib/api/types";
import { useSSHKeys } from "../../../../lib/hooks/use-ssh-keys";
import { repoHint, repoPattern } from "../../../../lib/repo-url";
import { TemplateIcon } from "./template-icon";

export const nameRe = /^[a-z][a-z0-9-]{0,30}$/;

type Props = { tpl: Template; name: string; setName: (v: string) => void; repo: string; setRepo: (v: string) => void; hosts: string[] };

export function DetailsStep({ tpl, name, setName, repo, setRepo, hosts }: Props) {
  const repoRe = repoPattern(hosts);
  const keys = useSSHKeys().data?.items.length ?? 0;
  return (
    <div className="grid gap-6 lg:grid-cols-[1fr_280px]">
      <div className="space-y-5">
        <TextField
          label="Workspace name"
          value={name}
          onChange={setName}
          isRequired
          autoFocus
          description="Lowercase letters, digits and dashes. Used in its HTTPS and preview addresses."
          validate={(v) => (nameRe.test(v) ? null : "Start with a letter; use a-z, 0-9 and dashes (max 31).")}
        />
        <TextField
          label="Git repository (optional)"
          value={repo}
          onChange={setRepo}
          placeholder="https://github.com/owner/repo"
          mono
          description="Public HTTPS clone for now. Cloned into ~/code, then the template's startup script runs."
          validate={(v) => (!v || repoRe.test(v.trim()) ? null : repoHint(hosts))}
        />
        <div className="flex items-center gap-2 text-sm text-fg-muted">
          <KeyRound className="size-4 text-hue-secrets" />
          {keys ? `${keys} SSH key${keys > 1 ? "s" : ""} from your account will be added.` : "No SSH keys yet. Add them under Account › SSH keys (optional)."}
        </div>
      </div>
      <aside className="h-fit rounded-xl border border-line bg-raised/40 p-4 text-sm">
        <div className="flex items-center gap-3">
          <TemplateIcon icon={tpl.icon} size={32} />
          <div className="font-medium text-fg">{tpl.name}</div>
        </div>
        <dl className="mt-3 space-y-1.5 text-fg-muted">
          <div className="flex justify-between"><dt>Toolchain</dt><dd className="text-right text-fg">{tpl.toolchain.join(", ")}</dd></div>
          <div className="flex justify-between"><dt>Previews</dt><dd className="text-fg">{tpl.ports.length ? tpl.ports.map((p) => `:${p}`).join(" ") : "—"}</dd></div>
          {tpl.repo && <div className="flex items-center gap-1.5 truncate"><GitBranch className="size-3.5" />{tpl.repo}</div>}
        </dl>
      </aside>
    </div>
  );
}
