import { useState } from "react";
import { Form } from "react-aria-components";
import { callAction } from "../../../../lib/api/actions";
import type { GithubRepository } from "../../../../lib/api/actions.gen";
import { Button } from "../../../../lib/design/components/button/button";
import { ModalDialog } from "../../../../lib/design/components/dialog/modal-dialog";
import { Select } from "../../../../lib/design/components/select/select";
import { Switch } from "../../../../lib/design/components/switch/switch";
import { useProjects } from "../../../../lib/hooks/use-projects";
import type { OrganizationWorkspace } from "../../../organizations/workspaces/use-organization-workspaces";
import { assignmentBody, assignmentFor, assignmentProblem, problemText, type Assignment } from "../model";
import { useRunnerChange } from "../use-runners";

const wholeWorkspace = "__workspace";

type Props = { tenantId: string; repository: GithubRepository; workspaces: OrganizationWorkspace[]; onClose: () => void };

/** Assign a repository to one of the organization's workspaces, optionally one of its projects, and allow
 * or stop its jobs on Shippin runners. The server checks the same rules and GitHub's reach again. */
export function AssignDialog({ tenantId, repository, workspaces, onClose }: Props) {
  const [a, setA] = useState<Assignment>(() =>
    assignmentFor(repository, workspaces.find((w) => w.is_default)?.id ?? workspaces[0]?.id ?? ""),
  );
  const projects = useProjects(tenantId, a.workspaceId || undefined);
  const save = useRunnerChange(tenantId, repository.workspace_id ? "Assignment saved" : "Repository assigned");
  const inWorkspace = (projects.data ?? []).filter((p) => p.workspace_id === a.workspaceId);
  const problem = projects.isPending && a.projectId ? null : assignmentProblem(a, workspaces, inWorkspace);
  return (
    <ModalDialog
      isOpen
      onOpenChange={(o) => {
        if (!o && !save.isPending) onClose();
      }}
      title={repository.workspace_id ? "Change assignment" : "Assign repository"}
      description={<span className="font-mono">{repository.full_name}</span>}
    >
      <Form
        className="space-y-4 px-6 py-5"
        onSubmit={(e) => {
          e.preventDefault();
          if (problem) return;
          save.mutate(
            () =>
              callAction(
                "shippin.integrations.github_repository.update",
                { tenantId, githubRepositoryId: repository.id },
                assignmentBody(a),
              ),
            { onSuccess: onClose },
          );
        }}
      >
        {workspaces.length === 0 ? (
          <p className="text-base text-fg-muted">This organization has no workspaces yet. Create one under Workspaces first.</p>
        ) : (
          <Select
            label="Workspace"
            value={a.workspaceId}
            onChange={(v) => setA({ ...a, workspaceId: v, projectId: "" })}
            options={workspaces.map((w) => ({ id: w.id, label: w.name }))}
          />
        )}
        <Select
          label="Project (optional)"
          value={a.projectId || wholeWorkspace}
          isDisabled={!a.workspaceId || projects.isPending}
          onChange={(v) => setA({ ...a, projectId: v === wholeWorkspace ? "" : v })}
          options={[{ id: wholeWorkspace, label: "The whole workspace" }, ...inWorkspace.map((p) => ({ id: p.id, label: p.name }))]}
        />
        <Switch
          isSelected={a.runners}
          onChange={(v) => setA({ ...a, runners: v })}
          tooltip="When off, the repository stays assigned but its jobs get no Shippin runner"
        >
          Allow Shippin runners for this repository's jobs
        </Switch>
        {problem && a.workspaceId && (
          <p role="alert" className="text-base text-danger">
            {problem}
          </p>
        )}
        {save.isError && (
          <p role="alert" className="text-base text-danger">
            {problemText(save.error)}
          </p>
        )}
        <div className="flex justify-end gap-2 border-t border-line pt-4">
          <Button variant="ghost" tooltip="Close without saving" onPress={onClose} isDisabled={save.isPending}>
            Cancel
          </Button>
          <Button
            type="submit"
            variant="primary"
            tooltip={problem ?? "Save where this repository belongs"}
            isDisabled={!!problem || save.isPending}
          >
            Save
          </Button>
        </div>
      </Form>
    </ModalDialog>
  );
}
