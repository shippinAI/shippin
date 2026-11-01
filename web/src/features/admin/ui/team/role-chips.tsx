import { Badge } from "../../../../lib/design/components/badge/badge";
import type { StaffRole } from "../../../../lib/api/team-types";

/** Role names for a member (unknown keys shown as-is, muted). */
export function RoleChips({ keys, roles }: { keys: string[]; roles: StaffRole[] }) {
  if (!keys.length) return <span className="text-fg-subtle">No role</span>;
  return (
    <div className="flex flex-wrap gap-1">
      {keys.map((k) => {
        const r = roles.find((x) => x.key === k);
        const owner = r?.permissions.includes("team.write");
        return (
          <Badge key={k} tone={!r ? "neutral" : owner ? "warning" : "primary"}>
            {r?.name ?? k}
          </Badge>
        );
      })}
    </div>
  );
}
