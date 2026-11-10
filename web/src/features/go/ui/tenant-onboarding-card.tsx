import { ArrowRight, Boxes } from "lucide-react";
import { LinkButton } from "../../../lib/design/components/button/link-button";
import { Card } from "../../../lib/design/components/card/card";

/** Shown on an empty tenant instead of a blank page: every feature lives in a workspace. */
export function TenantOnboardingCard({ tenantId, feature }: { tenantId: string; feature?: string }) {
  return (
    <Card className="flex items-start gap-4 px-6 py-5">
      <span className="grid size-10 shrink-0 place-items-center rounded-lg bg-raised"><Boxes className="size-5" strokeWidth={1.75} /></span>
      <div className="flex-1">
        <h2 className="text-lg font-semibold">Start with a workspace</h2>
        <p className="mt-1 text-sm text-fg-muted">
          {feature ? `${feature} runs inside a workspace. ` : ""}Your tenant has no workspaces yet. Create one to get a terminal,
          connected AI CLIs, guides, one-click Publish and usage numbers.
        </p>
      </div>
      <LinkButton href={`/t/${tenantId}/workspaces/new`} size="sm" tooltip="Create your first workspace">
        Create workspace <ArrowRight className="size-4" />
      </LinkButton>
    </Card>
  );
}
