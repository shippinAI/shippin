import { ArrowLeft, ArrowRight, Rocket } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "../../../../lib/router/router";
import { Button } from "../../../../lib/design/components/button/button";
import { LinkButton } from "../../../../lib/design/components/button/link-button";
import { Card } from "../../../../lib/design/components/card/card";
import { Skeleton } from "../../../../lib/design/components/skeleton/skeleton";
import { useTemplates } from "../../../../lib/hooks/use-templates";
import { useTenant } from "../../../../lib/hooks/use-tenant";
import { useCreateWorkspace } from "../../../../lib/hooks/use-workspaces";
import { PageHeader } from "../../../../template/page-header";
import { repoPattern } from "../../../../lib/repo-url";
import { DetailsStep, nameRe } from "./details-step";
import { SizePicker } from "./size-picker";
import { Stepper } from "./stepper";
import { TemplateGallery } from "./template-gallery";

const steps = ["Template", "Details", "Size"];

/** Template gallery → name/repo → size → create, then the detail page shows live progress. */
export function CreateWorkspacePage() {
  const { tenant } = useTenant();
  const tid = tenant?.tenant_id ?? "";
  const q = useTemplates();
  const create = useCreateWorkspace(tid);
  const nav = useNavigate();
  const [step, setStep] = useState(0);
  const [tplId, setTpl] = useState<string>();
  const [name, setName] = useState("");
  const [repo, setRepo] = useState("");
  const tpl = q.data?.items.find((t) => t.id === tplId);
  const sizes = q.data?.sizes ?? [];
  const hosts = q.data?.limits.repo_hosts ?? [];
  const def = sizes.find((z) => tpl && z.cpu === tpl.cpu && z.memory_gib === tpl.memory_gib)?.id ?? sizes[0]?.id ?? "";
  const [size, setSize] = useState("");
  const ok = [!!tpl, nameRe.test(name) && (!repo || repoPattern(hosts).test(repo.trim())), true][step];
  const submit = () =>
    create.mutate(
      { name, template_id: tplId, repo: repo.trim() || undefined, size: size || def },
      { onSuccess: (w) => nav(`/t/${tid}/workspaces/${w.id}`) },
    );
  return (
    <>
      <div className="mb-4">
        <LinkButton href={`/t/${tid}/workspaces`} variant="ghost" size="sm" tooltip="Back to all workspaces"><ArrowLeft className="size-4" /> Workspaces</LinkButton>
      </div>
      <PageHeader eyebrow={tenant?.name} title="New workspace" description="Pick a template, point it at a repo, choose a size." />
      <Stepper steps={steps} current={step} />
      <Card className="p-6">
        {q.isPending && <div className="grid gap-3 sm:grid-cols-2">{[0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-40" />)}</div>}
        {q.error && <p className="text-base text-danger">Templates couldn't load: {q.error.message}</p>}
        {q.data && step === 0 && <TemplateGallery templates={q.data.items} value={tplId} onChange={(id) => { setTpl(id); if (!name) setName(q.data.items.find((t) => t.id === id)?.slug.split("-")[0] ?? "dev"); }} />}
        {tpl && step === 1 && <DetailsStep tpl={tpl} name={name} setName={setName} repo={repo} setRepo={setRepo} hosts={hosts} />}
        {tpl && step === 2 && <SizePicker sizes={sizes} value={size || def} onChange={setSize} recommended={def} />}
        <div className="mt-6 flex items-center justify-between border-t border-line pt-4">
          <Button variant="ghost" isDisabled={step === 0} tooltip="Previous step" onPress={() => setStep(step - 1)}><ArrowLeft className="size-4" /> Back</Button>
          {step < 2 ? (
            <Button variant="primary" isDisabled={!ok} tooltip={ok ? "Continue" : "Complete this step first"} onPress={() => setStep(step + 1)}>Next <ArrowRight className="size-4" /></Button>
          ) : (
            <Button variant="primary" isDisabled={create.isPending} tooltip="Create the workspace and watch it provision" onPress={submit}>
              <Rocket className="size-4" /> {create.isPending ? "Creating…" : "Create workspace"}
            </Button>
          )}
        </div>
      </Card>
    </>
  );
}
