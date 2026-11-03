import { Sparkles } from "lucide-react";
import { useState } from "react";
import { Form, Label, ToggleButton, ToggleButtonGroup, type Key } from "react-aria-components";
import { useNavigate } from "../../../lib/router/router";
import type { PromptOptions } from "../../../lib/api/build-types";
import { Button } from "../../../lib/design/components/button/button";
import { TextField } from "../../../lib/design/components/field/text-field";
import { Select } from "../../../lib/design/components/select/select";
import { useCreateFromPrompt } from "../../../lib/hooks/use-build";
import { useSubscriptions } from "../../../lib/hooks/use-subscriptions";
import { surfaceKey } from "./devspace-rail";

const chip =
  "focus-ring cursor-default rounded-full border border-line px-3 py-1 text-xs text-fg-muted outline-none data-hovered:border-line-strong data-hovered:text-fg data-selected:border-primary data-selected:bg-primary/12 data-selected:text-primary";

/**
 * Describe it (M4, implementation-plan-3 "Prompt tab"): one box, Lovable/Bolt style. Shippin creates
 * the project and an AI devspace on your private server, then runs YOUR signed-in CLI there.
 */
export function PromptProjectForm({ tenantId, options }: { tenantId: string; options: PromptOptions }) {
  const nav = useNavigate();
  const create = useCreateFromPrompt(tenantId);
  const subs = useSubscriptions();
  const [prompt, setPrompt] = useState("");
  const [stack, setStack] = useState("auto");
  const [provider, setProvider] = useState(options.agents[0]?.id ?? "");
  const bytes = new TextEncoder().encode(prompt).length;
  const connected = (id: string) => subs.data?.connections.some((c) => c.provider === id && c.tenant_id === tenantId && c.status === "connected");
  const agents = options.agents.map((a) => ({ id: a.id, label: `${a.label}${connected(a.id) ? " · signed in" : ""}` }));
  const stacks = [{ template: "auto", label: "Pick for me" }, ...options.stacks];
  return (
    <Form
      className="space-y-4 p-5 pt-0"
      onSubmit={(e) => {
        e.preventDefault();
        create.mutate(
          { prompt, provider, stack: stack === "auto" ? undefined : stack },
          {
            onSuccess: (p) => {
              localStorage.setItem(surfaceKey(p.project_id), "build");
              nav(`/t/${tenantId}/projects/${p.project_id}`);
            },
          },
        );
      }}
    >
      <TextField label="What do you want to build?" multiline value={prompt} onChange={setPrompt} isRequired
        placeholder="A habit tracker with a weekly streak chart and a dark mode toggle"
        description={`${bytes.toLocaleString()} / ${options.max_bytes.toLocaleString()} bytes. Shippin passes this to your agent and keeps only a short preview.`}
        validate={() => (bytes > options.max_bytes ? "That's a bit long. Trim it a little." : null)} />
      <div className="flex flex-col gap-1.5">
        <Label id="stack-label" className="text-[13px] font-medium text-fg">Stack</Label>
        <ToggleButtonGroup aria-labelledby="stack-label" selectionMode="single" disallowEmptySelection selectedKeys={[stack]}
          onSelectionChange={(k: Set<Key>) => k.size && setStack(String([...k][0]))} className="flex flex-wrap gap-2">
          {stacks.map((s) => <ToggleButton key={s.template} id={s.template} className={chip}>{s.label}</ToggleButton>)}
        </ToggleButtonGroup>
      </div>
      {agents.length > 0 && <Select label="Built by" options={agents} value={provider} onChange={setProvider} />}
      {provider && !connected(provider) && (
        <p className="text-xs text-fg-subtle">You're not signed in to this one yet. Start anyway: the build waits and you can sign in from the devspace.</p>
      )}
      <Button type="submit" variant="primary" size="sm" isDisabled={create.isPending || !provider || bytes === 0}
        tooltip="Create the project and start building it in your devspace">
        <Sparkles className="size-4" /> Build it
      </Button>
    </Form>
  );
}
