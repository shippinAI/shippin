import { useEffect, useState } from "react";
import { Tab, TabList, TabPanel, Tabs } from "react-aria-components";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { CreateProjectForm } from "./create-project-form";
import { usePromptOptions } from "../../../lib/hooks/use-build";
import { GitHubImportForm } from "./github-import-form";
import { PromptProjectForm } from "./prompt-project-form";

const tab = "focus-ring relative -mb-px cursor-default border-b-2 border-transparent pb-2.5 text-sm text-fg-muted outline-none data-hovered:text-fg data-selected:border-primary data-selected:text-fg";

/** New project: describe it (M4, the default when enabled), start blank, or import from GitHub (?source=github). */
export function NewProjectPanel({ tenantId }: { tenantId: string }) {
  const source = new URLSearchParams(window.location.search).get("source");
  const prompt = usePromptOptions(tenantId);
  const canPrompt = !!prompt.data?.enabled;
  const [sel, setSel] = useState<string>(source === "github" ? "github" : "blank");
  useEffect(() => {
    if (canPrompt && !source) setSel("prompt");
  }, [canPrompt, source]);
  return (
    <Card elevation={2} className="self-start">
      <CardHeader title="New project" description="We place it on your private server automatically." />
      <Tabs selectedKey={sel} onSelectionChange={(k) => setSel(String(k))}>
        <TabList aria-label="New project source" className="mx-5 mb-4 flex gap-5 border-b border-line">
          {canPrompt && <Tab id="prompt" className={tab}>Describe it</Tab>}
          <Tab id="blank" className={tab}>Blank</Tab>
          <Tab id="github" className={tab}>Import from GitHub</Tab>
        </TabList>
        {canPrompt && prompt.data && <TabPanel id="prompt" className="outline-none"><PromptProjectForm tenantId={tenantId} options={prompt.data} /></TabPanel>}
        <TabPanel id="blank" className="outline-none"><CreateProjectForm tenantId={tenantId} /></TabPanel>
        <TabPanel id="github" className="outline-none"><GitHubImportForm tenantId={tenantId} active={sel === "github"} /></TabPanel>
      </Tabs>
    </Card>
  );
}
