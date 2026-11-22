import { useMemo, useState } from "react";
import type { SettingsView } from "../../../../lib/api/actions.gen";
import { Card, CardHeader } from "../../../../lib/design/components/card/card";
import { Skeleton } from "../../../../lib/design/components/skeleton/skeleton";
import { ago, historyEntries, problemOf } from "./model";
import { useSettingHistory } from "./use-settings";

/** A section's saved versions, newest first: who, when, why and what changed. Keys are named, never shown. */
export function SectionHistory({ view }: { view: SettingsView }) {
  const q = useSettingHistory(view.key, view.version > 0);
  const [now] = useState(() => Date.now());
  const entries = useMemo(() => historyEntries(view, q.data?.items ?? []), [view, q.data]);
  return (
    <Card>
      <CardHeader title="History" description="Every save of this section, with who made it and why." />
      <div className="px-5 py-4 text-sm">
        {view.version === 0 ? (
          <p className="text-fg-muted">Not saved in the panel yet, so there is no history.</p>
        ) : q.isError ? (
          <p role="alert" className="text-danger">
            Couldn't load the history. {problemOf(q.error).message}
          </p>
        ) : q.isPending ? (
          <Skeleton className="h-16 w-full" />
        ) : entries.length === 0 ? (
          <p className="text-fg-muted">No saved versions are listed.</p>
        ) : (
          <ol className="space-y-4" aria-label={`${view.title} history`}>
            {entries.map((e) => (
              <li key={e.version} className="border-l-2 border-line pl-3">
                <p className="text-fg">
                  <span className="font-medium">Version {e.version}</span>
                  <span className="text-fg-muted">{`, ${ago(e.at, now)}${e.by ? ` by ${e.by}` : ""}`}</span>
                </p>
                {e.at && <p className="text-xs text-fg-subtle">{new Date(e.at).toLocaleString()}</p>}
                {e.reason && <p className="mt-1 text-fg-muted">Why: {e.reason}</p>}
                {e.changes.length > 0 ? (
                  <ul className="mt-1 list-disc space-y-0.5 pl-5 text-fg-muted">
                    {e.changes.map((c) => (
                      <li key={c} className="break-words">
                        {c}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="mt-1 text-xs text-fg-subtle">No value changed.</p>
                )}
              </li>
            ))}
          </ol>
        )}
      </div>
    </Card>
  );
}
