import { GitBranchPlus } from "lucide-react";
import { useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { ModalDialog } from "../../../lib/design/components/dialog/modal-dialog";
import { toast } from "../../../lib/design/components/toast/toast-queue";
import { useGitHubRepos, usePublishProject } from "../../../lib/hooks/use-github";
import type { FoundFolder } from "../../../lib/hooks/use-discover";
import { GitHubRepoPicker, type PickedRepo } from "./github-repo-picker";

/** One click: create a private GitHub repo named after the project and push the folder. Personal
 *  accounts can't get an App-created repo, so they link an empty one instead (same dialog). */
export function PublishFolder({ tenantId, f }: { tenantId: string; f: FoundFolder }) {
  const [open, setOpen] = useState(false);
  const [needLink, setNeedLink] = useState(false);
  const [picked, setPicked] = useState<PickedRepo>();
  const repos = useGitHubRepos(tenantId, open);
  const pub = usePublishProject(tenantId);
  const inst = repos.data?.installations.find((i) => !i.suspended);
  const send = (repo_id?: number) => {
    if (!f.project_id || !inst) return;
    pub.mutate({ projectId: f.project_id, installation_id: picked?.installation ?? inst.installation_id, repo_id }, {
      onSuccess: () => setOpen(false),
      onError: (e) => {
        if (!repo_id && e.message.includes("only in organizations")) setNeedLink(true);
        else toast.error("Not synced", e.message);
      },
    });
  };
  return (
    <>
      <Button size="sm" variant="secondary" tooltip="Create a private GitHub repo and push this folder" onPress={() => setOpen(true)}>
        <GitBranchPlus className="size-4" /> Create GitHub repo &amp; sync
      </Button>
      <ModalDialog isOpen={open} onOpenChange={setOpen} title={`Put ${f.slug} on GitHub`}
        description={inst ? `Shippin creates a private repository ${inst.account_login}/${f.slug} and pushes ${f.path.replace("/home/dev/", "~/")}. Dependencies and .env files are left out.` : "Connect GitHub first (Import from GitHub tab)."}>
        <div className="space-y-4 p-6">
          {needLink && inst && (
            <>
              <p className="text-sm text-fg-muted">
                {inst.account_login} is a personal account, where GitHub doesn't let Apps create repositories.{" "}
                <a className="text-primary underline" target="_blank" rel="noreferrer" href={`https://github.com/new?name=${f.slug}&visibility=private`}>Create an empty repo named {f.slug}</a>, give Shippin access, then link it:
              </p>
              <GitHubRepoPicker installations={repos.data?.installations ?? []} value={picked} onChange={setPicked} />
            </>
          )}
          <div className="flex justify-end gap-2">
            <Button variant="ghost" size="sm" tooltip="Close without changes" onPress={() => setOpen(false)}>Cancel</Button>
            {needLink ? (
              <Button variant="primary" size="sm" tooltip="Push this folder to the picked repository" isDisabled={!picked} isPending={pub.isPending} onPress={() => send(picked?.repo.id)}>Link &amp; push</Button>
            ) : (
              <Button variant="primary" size="sm" tooltip="Create the private repository and push" isDisabled={!inst} isPending={pub.isPending} onPress={() => send()}>Create &amp; sync</Button>
            )}
          </div>
        </div>
      </ModalDialog>
    </>
  );
}
