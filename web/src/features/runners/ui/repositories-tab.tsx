import { ExternalLink, Lock, Pencil, Unlink } from "lucide-react";
import { useState } from "react";
import { callAction } from "../../../lib/api/actions";
import type { GithubRepository } from "../../../lib/api/actions.gen";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { IconButton } from "../../../lib/design/components/button/icon-button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { DataTable, type ColumnDef } from "../../../lib/design/components/data-table/data-table";
import { ConfirmDialog } from "../../../lib/design/components/dialog/confirm-dialog";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";
import { githubInstallURL } from "../../../lib/hooks/use-github";
import { useProjects } from "../../../lib/hooks/use-projects";
import { useOrganizationWorkspaces } from "../../workspaces/use-organization-workspaces";
import { problemText, repositoryState } from "../model";
import { useGitHubConnections, useRunnerChange, useRunnerRepositories } from "../use-runners";
import { AssignDialog } from "./assign-dialog";

type Props = { tenantId: string; canManage: boolean };

/** The organization's GitHub repositories: connect GitHub, then (managers) assign each to a workspace or
 * project and allow its jobs on Shippin runners. Everyone else reads. */
export function RepositoriesTab({ tenantId, canManage }: Props) {
  const repos = useRunnerRepositories(tenantId);
  const conn = useGitHubConnections(tenantId);
  const workspaces = useOrganizationWorkspaces(tenantId);
  const projects = useProjects(tenantId);
  const unassign = useRunnerChange(tenantId, "Repository unassigned");
  const [editing, setEditing] = useState<GithubRepository>();
  const [removing, setRemoving] = useState<GithubRepository>();
  const installs = conn.data?.items ?? [];
  const canInstall = !!conn.data?.can_connect;
  const wsName = (id: string | null) => workspaces.data?.find((w) => w.id === id)?.name ?? (id ? "A removed workspace" : "");
  const projectName = (id: string | null) => projects.data?.find((p) => p.id === id)?.name ?? (id ? "A removed project" : "");
  const connect = canInstall && (
    <Button
      variant={installs.length ? "secondary" : "primary"}
      size="sm"
      tooltip="Install the Shippin GitHub App on GitHub and choose which repositories it can reach"
      onPress={() => window.location.assign(githubInstallURL(tenantId, "runners"))}
    >
      <ExternalLink className="size-4" aria-hidden /> {installs.length ? "Add repositories on GitHub" : "Connect GitHub"}
    </Button>
  );

  if (repos.isPending || conn.isPending) return <Skeleton className="h-40" />;
  if (repos.isError && (repos.error as { status?: number }).status === 503)
    return (
      <Card>
        <EmptyState title="GitHub isn't set up here yet" description={problemText(repos.error)} />
      </Card>
    );
  if (conn.data && installs.length === 0)
    return (
      <Card>
        <EmptyState
          title="Connect GitHub first"
          description={
            canInstall
              ? "Connect your GitHub account or organization with the Shippin GitHub App. You choose exactly which repositories Shippin can see, and disconnecting never touches the repositories themselves."
              : "Your organization hasn't connected GitHub yet. An owner or admin connects it, then repositories show up here."
          }
          action={connect}
        />
      </Card>
    );

  const columns: ColumnDef<GithubRepository>[] = [
    {
      id: "repo",
      label: "Repository",
      isRowHeader: true,
      render: (r) => (
        <span className="inline-flex items-center gap-1.5 font-mono text-sm text-fg">
          {r.full_name}
          {r.private && (
            <>
              <Lock className="size-3.5 text-fg-subtle" aria-hidden />
              <span className="sr-only">(private)</span>
            </>
          )}
        </span>
      ),
    },
    {
      id: "where",
      label: "Assigned to",
      render: (r) =>
        r.workspace_id ? (
          <span>
            {wsName(r.workspace_id)}
            {r.project_id && <span className="text-fg-subtle"> / {projectName(r.project_id)}</span>}
          </span>
        ) : (
          <span className="text-fg-subtle">Not assigned</span>
        ),
    },
    {
      id: "runners",
      label: "Shippin runners",
      render: (r) => {
        const s = repositoryState(r);
        return <Badge tone={s.tone}>{s.label}</Badge>;
      },
    },
    ...(canManage
      ? [
          {
            id: "actions",
            label: "Actions",
            width: "6rem",
            render: (r: GithubRepository) => (
              <div className="flex justify-end gap-1">
                <IconButton
                  icon={Pencil}
                  label={r.workspace_id ? `Change ${r.full_name}` : `Assign ${r.full_name}`}
                  onPress={() => setEditing(r)}
                />
                {r.workspace_id && (
                  <IconButton
                    icon={Unlink}
                    label={`Unassign ${r.full_name}`}
                    isDisabled={unassign.isPending}
                    onPress={() => setRemoving(r)}
                  />
                )}
              </div>
            ),
          },
        ]
      : []),
  ];

  return (
    <>
      <Card className="overflow-hidden">
        <CardHeader
          title="GitHub repositories"
          description={
            canManage
              ? "Assign a repository to a workspace, and optionally a project, then allow Shippin runners. Its jobs get runners only after that."
              : "Read only. An owner or admin assigns repositories and allows Shippin runners."
          }
          actions={connect}
        />
        {repos.isError && (
          <p role="alert" className="px-5 py-4 text-base text-danger">
            {problemText(repos.error)}
          </p>
        )}
        {unassign.isError && (
          <p role="alert" className="px-5 pt-4 text-base text-danger">
            {problemText(unassign.error)}
          </p>
        )}
        {installs.some((i) => i.suspended) && (
          <p className="px-5 pt-4 text-sm text-warning">
            A GitHub connection is suspended on GitHub; its repositories are left out until it is resumed there.
          </p>
        )}
        {repos.data && (
          <DataTable
            label="GitHub repositories"
            columns={columns}
            rows={repos.data.items}
            dense
            dependencies={[workspaces.data, projects.data, canManage, unassign.isPending]}
            empty={
              <p className="px-5 py-8 text-center text-base text-fg-muted">
                GitHub lists no repositories for this organization's connections. Choose repositories for the Shippin GitHub App on GitHub.
              </p>
            }
          />
        )}
      </Card>
      {editing && (
        <AssignDialog tenantId={tenantId} repository={editing} workspaces={workspaces.data ?? []} onClose={() => setEditing(undefined)} />
      )}
      <ConfirmDialog
        isOpen={!!removing}
        onOpenChange={(o) => {
          if (!o) setRemoving(undefined);
        }}
        title="Unassign repository"
        confirmLabel="Unassign"
        body={
          <p>
            New jobs from <span className="font-mono text-fg">{removing?.full_name}</span> will no longer get Shippin runners. The
            repository stays on GitHub, and its job history stays here.
          </p>
        }
        onConfirm={() => {
          const r = removing;
          if (r) unassign.mutate(() => callAction("shippin.integrations.github_repository.delete", { tenantId, githubRepositoryId: r.id }));
        }}
      />
    </>
  );
}
