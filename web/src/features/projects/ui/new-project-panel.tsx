import { useEffect, useState } from "react";
import { Tab, TabList, TabPanel, Tabs } from "react-aria-components";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { CreateProjectForm } from "./create-project-form";
import { usePromptOptions } from "../../../lib/hooks/use-build";
import { GitHubImportForm } from "./github-import-form";
import { PromptProjectForm } from "./prompt-project-form";
import { Select } from "../../../lib/design/components/select/select";
import { useOrganizationWorkspaces } from "../../workspaces/use-organization-workspaces";
import { useText } from "../../../lib/ui-config/use-ui-config";

const tab = "focus-ring relative -mb-px cursor-default border-b-2 border-transparent pb-2.5 text-base text-fg-muted outline-none data-hovered:text-fg data-selected:border-primary data-selected:text-fg";

/** New project: describe it (M4, the default when enabled), start blank, or import from GitHub (?source=github). */
export function NewProjectPanel({ tenantId, workspaceId }: { tenantId: string; workspaceId?: string }) {
  const groups = useOrganizationWorkspaces(tenantId);
  const [chosen, setChosen] = useState(workspaceId ?? "");
  const group = chosen || groups.data?.find((g) => g.is_default)?.id || "";
  const text = useText();
  const source = new URLSearchParams(window.location.search).get("source");
  const prompt = usePromptOptions(tenantId);
  const canPrompt = !!prompt.data?.enabled;
  const [sel, setSel] = useState<string>(source === "github" ? "github" : "blank");
  useEffect(() => {
    if (canPrompt && !source) setSel("prompt");
  }, [canPrompt, source]);
  return (
    <Card elevation={2} className="self-start">
      <CardHeader title="New project" description={text("spaces.newProject")} />
      {!workspaceId && <div className="px-5 pb-4"><Select label={text("spaces.select")} options={(groups.data ?? []).map((g) => ({ id: g.id, label: g.name }))} value={group} onChange={setChosen} /></div>}
      {groups.isError && <p role="alert" className="px-5 text-danger">{groups.error.message}</p>}
      <Tabs selectedKey={sel} onSelectionChange={(k) => setSel(String(k))}>
        <TabList aria-label="New project source" className="mx-5 mb-4 flex gap-5 border-b border-line">
          {canPrompt && <Tab id="prompt" className={tab}>Describe it</Tab>}
          <Tab id="blank" className={tab}>Blank</Tab>
          <Tab id="github" className={tab}>Import from GitHub</Tab>
        </TabList>
        {canPrompt && prompt.data && <TabPanel id="prompt" className="outline-none"><PromptProjectForm tenantId={tenantId} workspaceId={group} options={prompt.data} /></TabPanel>}
        <TabPanel id="blank" className="outline-none"><CreateProjectForm tenantId={tenantId} workspaceId={group} /></TabPanel>
        <TabPanel id="github" className="outline-none"><GitHubImportForm tenantId={tenantId} workspaceId={group} active={sel === "github"} /></TabPanel>
      </Tabs>
    </Card>
  );
}
