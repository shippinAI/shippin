import { AlertTriangle } from "lucide-react";
import type { ReactNode } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { Skeleton } from "../../../lib/design/components/skeleton/skeleton";

type Props = { isPending: boolean; error: Error | null; onRetry: () => void; rows?: number; children: () => ReactNode };

/** Loading skeleton / error with retry / content, for every provider panel section. */
export function QueryView({ isPending, error, onRetry, rows = 5, children }: Props) {
  if (isPending)
    return (
      <div className="space-y-2 p-5" aria-busy>
        {Array.from({ length: rows }, (_, i) => <Skeleton key={i} className="h-8 w-full" />)}
      </div>
    );
  if (error)
    return (
      <div className="flex items-start gap-3 p-5 text-base">
        <AlertTriangle className="mt-0.5 size-4 shrink-0 text-danger" />
        <div className="flex-1">
          <div className="font-medium text-fg">Couldn't load this section</div>
          <div className="mt-0.5 text-fg-muted">{error.message}</div>
        </div>
        <Button size="sm" variant="secondary" tooltip="Try the request again" onPress={onRetry}>Retry</Button>
      </div>
    );
  return <>{children()}</>;
}
