import { useState } from "react";
import { Play, Sparkles, Trash2 } from "lucide-react";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { IconButton } from "../../../lib/design/components/button/icon-button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { ConfirmDialog } from "../../../lib/design/components/dialog/confirm-dialog";
import { Disclosure } from "../../../lib/design/components/disclosure/disclosure";
import { TextField } from "../../../lib/design/components/field/text-field";
import { timeAgo } from "../../../lib/format";
import { DbError, useAskAI, useDbAudit, useDbSnippets, useDeleteSnippet, useRunSQL, useSaveSnippet } from "../../../lib/hooks/use-database";
import { hintForSQL } from "../model";

export function SqlPanel({ tenantId, readOnly }: { tenantId: string; readOnly: boolean }) {
  const [sql, setSql] = useState("select id, title, done\nfrom todos\norder by created_at desc");
  const [name, setName] = useState("");
  const [prompt, setPrompt] = useState("");
  const [confirm, setConfirm] = useState(false);
  const [error, setError] = useState("");
  const run = useRunSQL(tenantId);
  const snippets = useDbSnippets(tenantId);
  const save = useSaveSnippet(tenantId);
  const remove = useDeleteSnippet(tenantId);
  const audit = useDbAudit(tenantId);
  const ask = useAskAI(tenantId);

  const execute = (confirmed: boolean) => {
    setError("");
    run.mutate({ sql, read_only: readOnly, confirm: confirmed }, {
      onSuccess: () => setConfirm(false),
      onError: (e) => {
        if (e instanceof DbError && e.confirmationRequired) setConfirm(true);
        else setError(e.message);
      },
    });
  };

  const result = run.data;
  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-[1fr_280px]">
      <div className="space-y-4">
        <Card>
          <CardHeader title="SQL editor" description={hintForSQL(sql)} actions={
            <Button variant="primary" size="sm" tooltip={readOnly ? "Read-only allows SELECT" : "Run this statement"} isDisabled={!sql.trim() || run.isPending} onPress={() => execute(false)}>
              <Play className="size-3.5" /> Run
            </Button>
          } />
          <div className="p-4">
            <label className="sr-only" htmlFor="sql-editor">SQL</label>
            <textarea
              id="sql-editor"
              value={sql}
              onChange={(e) => setSql(e.target.value)}
              spellCheck={false}
              rows={8}
              className="focus-ring w-full resize-y rounded-[var(--radius-control)] border border-line bg-canvas px-3 py-2 font-mono text-[12.5px] leading-relaxed text-fg outline-none"
            />
            {error && <p role="alert" className="mt-3 text-[13px] text-danger">{error}</p>}
            {result?.truncated && <p className="mt-3 text-[12.5px] text-warning">Showing the first {result.row_count} rows. Add a tighter WHERE clause to see a different slice.</p>}
          </div>
          {result && (
            <div className="overflow-x-auto border-t border-line">
              <table className="w-full border-separate border-spacing-0 text-sm" aria-label="Query results">
                <thead>
                  <tr>
                    {(result.columns.length ? result.columns : ["result"]).map((c) => (
                      <th key={c} className="border-b border-line bg-surface px-3 py-2 text-left text-[11.5px] font-medium tracking-wide text-fg-subtle uppercase">{c}</th>
                    ))}
                  </tr>
                </thead>
                <tbody>
                  {result.rows.length === 0 && (
                    <tr><td className="px-4 py-6 text-[13px] text-fg-muted" colSpan={Math.max(1, result.columns.length)}>{result.kind === "read" ? "No rows." : `${result.row_count} affected.`}</td></tr>
                  )}
                  {result.rows.map((row, i) => (
                    <tr key={i}>
                      {row.map((cell, j) => (
                        <td key={j} className="border-b border-line px-3 py-2 font-mono text-[12.5px] text-fg">{cell == null ? <span className="text-fg-subtle">null</span> : String(cell)}</td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>
        <Card>
          <CardHeader title="Ask AI" description="A hook for this organization's AI subscription. Nothing is sent to a model yet." />
          <form className="space-y-3 p-4" onSubmit={(e) => { e.preventDefault(); ask.mutate(prompt); }}>
            <TextField label="What should the database do?" value={prompt} onChange={setPrompt} placeholder="A table of customers with email and a signed-up date" multiline />
            <Button type="submit" variant="secondary" size="sm" tooltip="Draft SQL later, via the AI subscription" isDisabled={!prompt.trim() || ask.isPending}>
              <Sparkles className="size-3.5" /> Ask AI
            </Button>
            {ask.data && (
              <div className="rounded-lg border border-line bg-canvas/50 px-3 py-2 text-[13px] text-fg-muted">
                <Badge tone="info">{ask.data.status}</Badge>
                <p className="mt-2">{ask.data.message}</p>
              </div>
            )}
            {ask.isError && <p className="text-[13px] text-danger">{(ask.error as Error).message}</p>}
          </form>
        </Card>
        <Disclosure title="SQL audit log">
          <ul className="max-h-72 overflow-y-auto">
            {(audit.data?.entries ?? []).length === 0 && <li className="px-5 py-4 text-[13px] text-fg-muted">Statements you run show up here, including ones that were refused.</li>}
            {(audit.data?.entries ?? []).map((e) => (
              <li key={e.id} className="border-b border-line px-5 py-3 last:border-b-0">
                <div className="flex flex-wrap items-center gap-2 text-[12px] text-fg-subtle">
                  <Badge tone={e.status === "ok" ? "success" : e.status === "rejected" ? "warning" : "danger"}>{e.status}</Badge>
                  <span>{e.action}</span>
                  <span>{e.actor_name}</span>
                  {e.created_at && <span>{timeAgo(e.created_at)}</span>}
                </div>
                <pre className="mt-1 overflow-x-auto font-mono text-[12px] text-fg">{e.statement}</pre>
                {e.error && <p className="mt-1 text-[12px] text-fg-muted">{e.error}</p>}
              </li>
            ))}
          </ul>
        </Disclosure>
      </div>
      <Card className="h-fit overflow-hidden">
        <CardHeader title="Saved snippets" description="Only you see these, inside this organization." />
        <form className="space-y-2 border-b border-line p-4" onSubmit={(e) => { e.preventDefault(); save.mutate({ name, sql }); }}>
          <TextField label="Name" value={name} onChange={setName} placeholder="Open todos" />
          <Button type="submit" size="sm" variant="secondary" tooltip="Save the statement in the editor" isDisabled={!name.trim() || !sql.trim()}>Save</Button>
        </form>
        <ul>
          {(snippets.data?.snippets ?? []).map((s) => (
            <li key={s.id} className="flex items-center gap-1 border-b border-line last:border-b-0">
              <button type="button" className="focus-ring min-w-0 flex-1 truncate px-4 py-2.5 text-left text-[13px] text-fg" onClick={() => setSql(s.sql)}>{s.name}</button>
              <IconButton icon={Trash2} label={`Delete ${s.name}`} onPress={() => remove.mutate(s.id)} />
            </li>
          ))}
          {snippets.data?.snippets.length === 0 && <li className="px-4 py-4 text-[12.5px] text-fg-subtle">Nothing saved yet.</li>}
        </ul>
      </Card>
      <ConfirmDialog
        isOpen={confirm}
        onOpenChange={setConfirm}
        title="Run this destructive statement?"
        body={<span>It can remove rows or change the shape of the table. Read-only mode is {readOnly ? "on, so it will still be refused." : "off."}</span>}
        confirmLabel="Run statement"
        onConfirm={() => execute(true)}
      />
    </div>
  );
}
