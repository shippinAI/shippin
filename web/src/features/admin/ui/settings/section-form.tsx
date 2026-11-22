import { useMemo, useState } from "react";
import { Link } from "react-aria-components";
import type { SettingsFieldView, SettingsView } from "../../../../lib/api/actions.gen";
import { Badge } from "../../../../lib/design/components/badge/badge";
import { Button } from "../../../../lib/design/components/button/button";
import { LinkButton } from "../../../../lib/design/components/button/link-button";
import { Card, CardHeader } from "../../../../lib/design/components/card/card";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { Notice } from "../../../../lib/design/components/notice/notice";
import { Select } from "../../../../lib/design/components/select/select";
import { Switch } from "../../../../lib/design/components/switch/switch";
import { useInfraOverview } from "../../../../lib/hooks/use-infra";
import {
  ago,
  type Draft,
  fieldOrigin,
  initialDraft,
  type Input,
  isKey,
  type KeyEdit,
  list,
  noneId,
  pickOptions,
  planSave,
  problemOf,
  toInput,
} from "./model";
import { ReasonDialog } from "./reason-dialog";
import { useSaveSetting } from "./use-settings";

type Props = { view: SettingsView; storeConnected: boolean; canWrite: boolean; onReload: () => void };

/** One settings section as a form generated from its fields. Mount it again (key) when the section's
 * version changes, so the form starts from what is saved. */
export function SectionForm({ view, storeConnected, canWrite, onReload }: Props) {
  const [draft, setDraft] = useState<Draft>(() => initialDraft(view));
  const plan = useMemo(() => planSave(view, draft, storeConnected), [view, draft, storeConnected]);
  const save = useSaveSetting(view.key);
  const [asking, setAsking] = useState(false);
  const [lastReason, setLastReason] = useState("");
  const [now] = useState(() => Date.now());
  const usesFleets = view.fields.some((f) => f.source === "fleets");
  const infra = useInfraOverview(usesFleets);
  const fleets = usesFleets && infra.data ? infra.data.fleets.map((x) => ({ id: x.id, label: x.label })) : null;

  const touch = () => save.isError && save.reset();
  const setInput = (name: string, v: Input) => {
    touch();
    setDraft((d) => ({ ...d, inputs: { ...d.inputs, [name]: v } }));
  };
  const setKey = (name: string, e: KeyEdit | null) => {
    touch();
    setDraft((d) => {
      const keys = { ...d.keys };
      if (e) keys[name] = e;
      else delete keys[name];
      return { ...d, keys };
    });
  };

  const errors = Object.keys(plan.errors).length;
  const dirty = plan.changed.length > 0;
  const canSave = canWrite && dirty && errors === 0 && !plan.blocked && !save.isPending;
  const problem = save.isError ? problemOf(save.error) : null;
  const disabled = !canWrite || save.isPending;
  const values = plan.body.values ?? {};

  return (
    <Card>
      <CardHeader
        title={view.title}
        description={view.summary}
        actions={<Badge tone={view.version > 0 ? "primary" : "neutral"}>{view.version > 0 ? "Saved in the panel" : "From the file"}</Badge>}
      />
      <p className="border-b border-line px-5 py-2.5 text-xs text-fg-subtle">
        {view.version > 0
          ? `Version ${view.version}, changed ${ago(view.updated_at, now)}${view.updated_by ? ` by ${view.updated_by}` : ""}. The bootstrap file no longer applies to this section.`
          : "Not saved in the panel yet: these values come from the bootstrap file. Your first save makes the panel's values the ones in force."}
      </p>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (canSave) setAsking(true);
        }}
      >
        {!canWrite && (
          <Notice tone="info" className="mx-5 mt-4">
            Your staff role can read these settings but not change them. Changing them needs the platform.settings.write permission.
          </Notice>
        )}
        <ul className="divide-y divide-line" aria-label={`${view.title} fields`}>
          {view.fields.map((f) => (
            <li key={f.name} className="px-5 py-4">
              {isKey(f) ? (
                <KeyRow
                  f={f}
                  view={view}
                  edit={draft.keys[f.name]}
                  onEdit={(e) => setKey(f.name, e)}
                  storeConnected={storeConnected}
                  disabled={disabled}
                  error={plan.errors[f.name]}
                />
              ) : (
                <ValueRow
                  f={f}
                  view={view}
                  input={draft.inputs[f.name] ?? toInput(f, f.value)}
                  onChange={(v) => setInput(f.name, v)}
                  disabled={disabled}
                  error={plan.errors[f.name]}
                  changed={f.name in values}
                  fleets={f.source === "fleets" ? fleets : null}
                />
              )}
            </li>
          ))}
        </ul>
        {canWrite && (
          <div className="space-y-3 border-t border-line px-5 py-4">
            {plan.blocked && dirty && (
              <Notice
                tone="warning"
                title="Connect the secret store first"
                actions={
                  <LinkButton href="/admin/secret-store" size="sm" variant="secondary" tooltip="Connect this installation to Vaultix">
                    Open Secret store
                  </LinkButton>
                }
              >
                {plan.blocked}
              </Notice>
            )}
            {problem && (
              <Notice
                tone="danger"
                title="Not saved"
                actions={
                  problem.kind === "stale" ? (
                    <Button size="sm" variant="secondary" tooltip="Load the saved section; your edits here are cleared" onPress={onReload}>
                      Reload
                    </Button>
                  ) : problem.kind === "no_store" ? (
                    <LinkButton href="/admin/secret-store" size="sm" variant="secondary" tooltip="Connect this installation to Vaultix">
                      Open Secret store
                    </LinkButton>
                  ) : null
                }
              >
                {problem.message}
              </Notice>
            )}
            <div className="flex flex-wrap items-center justify-end gap-2">
              <p className="mr-auto text-sm text-fg-muted" aria-live="polite">
                {errors > 0
                  ? `Fix ${errors === 1 ? "the field" : `${errors} fields`} marked above to save.`
                  : dirty
                    ? `Changed: ${list(plan.changed)}.`
                    : "No changes yet."}
              </p>
              <Button
                variant="ghost"
                isDisabled={!dirty && errors === 0}
                onPress={() => {
                  save.reset();
                  setDraft(initialDraft(view));
                }}
                tooltip="Put every field back to what is saved"
              >
                Discard changes
              </Button>
              <Button
                type="submit"
                variant="primary"
                isDisabled={!canSave}
                tooltip={
                  !dirty
                    ? "Change something first"
                    : errors
                      ? "Fix the marked fields first"
                      : (plan.blocked ?? "Save these changes, with a reason")
                }
              >
                {save.isPending ? "Saving…" : "Save…"}
              </Button>
            </div>
          </div>
        )}
      </form>
      {asking && (
        <ReasonDialog
          title={`Save ${view.title}?`}
          description="It takes effect straight away, without a restart, and is kept in this section's history."
          confirmLabel="Save"
          pending={save.isPending}
          initialReason={lastReason}
          onCancel={() => setAsking(false)}
          onConfirm={(reason) => {
            setLastReason(reason);
            save.mutate({ patch: plan.body, reason }, { onSettled: () => setAsking(false) });
          }}
        >
          <div>
            <p className="text-sm font-medium text-fg">What changes</p>
            <ul className="mt-1 list-disc space-y-0.5 pl-5 text-sm">
              {plan.changed.map((c) => (
                <li key={c}>{c}</li>
              ))}
            </ul>
          </div>
          {plan.moved.length > 0 && (
            <Notice tone="info">
              {list(plan.moved)} {plan.moved.length === 1 ? "moves" : "move"} from the panel's environment into Vaultix with this first
              save. After it, the panel reads {plan.moved.length === 1 ? "it" : "them"} only from Vaultix.
            </Notice>
          )}
          {view.version === 0 && (
            <p className="text-sm">
              This is the section's first save in the panel: from now on the panel's values are the ones in force, not the bootstrap file's.
            </p>
          )}
        </ReasonDialog>
      )}
    </Card>
  );
}

function Origin({ view, f, changed }: { view: SettingsView; f: SettingsFieldView; changed?: boolean }) {
  const o = fieldOrigin(view, f);
  return (
    <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-fg-subtle">
      <Badge tone={o.tone}>{o.label}</Badge>
      <span>{o.hint}</span>
      {changed && <Badge tone="info">Changed, not saved</Badge>}
    </div>
  );
}

type ValueProps = {
  f: SettingsFieldView;
  view: SettingsView;
  input: Input;
  onChange: (v: Input) => void;
  disabled: boolean;
  error?: string;
  changed: boolean;
  fleets: { id: string; label: string }[] | null;
};

function ValueRow({ f, view, input, onChange, disabled, error, changed, fleets }: ValueProps) {
  const errId = `setting-${view.key}-${f.name}-error`;
  const text = typeof input === "string" ? input : "";
  const invalid = { isInvalid: !!error, "aria-describedby": error ? errId : undefined };
  let control;
  if (f.kind === "bool") {
    control = (
      <div className="flex flex-col gap-1.5">
        <span className="text-sm font-medium text-fg">{f.label}</span>
        {f.help && <span className="text-xs text-fg-subtle">{f.help}</span>}
        <div>
          <Switch
            aria-label={f.label}
            isSelected={input === true}
            onChange={onChange}
            isDisabled={disabled}
            tooltip={input === true ? "On; turn it off" : "Off; turn it on"}
          >
            {input === true ? "On" : "Off"}
          </Switch>
        </div>
      </div>
    );
  } else if (f.kind === "choice" && f.choices?.length) {
    control = (
      <>
        <Select
          label={f.label}
          value={text}
          onChange={onChange}
          isDisabled={disabled}
          options={f.choices.map((c) => ({ id: c.value, label: c.label }))}
        />
        {f.help && <p className="mt-1.5 text-xs text-fg-subtle">{f.help}</p>}
      </>
    );
  } else if (fleets) {
    control = (
      <>
        <Select
          label={f.label}
          value={text || (f.optional ? noneId : "")}
          onChange={(v) => onChange(v === noneId ? "" : v)}
          isDisabled={disabled}
          options={pickOptions(fleets, text, !!f.optional)}
        />
        {f.help && <p className="mt-1.5 text-xs text-fg-subtle">{f.help}</p>}
      </>
    );
  } else {
    const multiline = f.kind === "long_text" || f.kind === "list" || f.kind === "map";
    const how = f.kind === "list" ? "One per line." : f.kind === "map" ? "One per line, as name=value." : "";
    const bounds =
      f.kind === "integer" ? (f.max ? `A whole number from ${f.min ?? 0} to ${f.max}.` : `A whole number, at least ${f.min ?? 0}.`) : "";
    const description = [f.help, how, bounds, f.optional ? "May be left empty." : ""].filter(Boolean).join(" ");
    control = (
      <TextField
        label={f.label}
        value={text}
        onChange={onChange}
        isDisabled={disabled}
        multiline={multiline}
        mono={f.kind === "map" || f.kind === "list"}
        type={f.kind === "integer" ? "number" : f.kind === "url" ? "url" : "text"}
        min={f.kind === "integer" ? f.min : undefined}
        max={f.kind === "integer" && f.max ? f.max : undefined}
        step={f.kind === "integer" ? 1 : undefined}
        placeholder={f.kind === "url" ? "https://" : undefined}
        description={description || undefined}
        {...invalid}
      />
    );
  }
  return (
    <>
      {control}
      {error && (
        <p id={errId} className="mt-1.5 text-xs text-danger">
          {error}
        </p>
      )}
      <Origin view={view} f={f} changed={changed} />
    </>
  );
}

type KeyProps = {
  f: SettingsFieldView;
  view: SettingsView;
  edit: KeyEdit | undefined;
  onEdit: (e: KeyEdit | null) => void;
  storeConnected: boolean;
  disabled: boolean;
  error?: string;
};

/** A key is never shown. The row says whether one is saved and where, and offers to replace, move or remove it. */
function KeyRow({ f, view, edit, onEdit, storeConnected, disabled, error }: KeyProps) {
  const errId = `setting-${view.key}-${f.name}-error`;
  const where = f.key?.in ?? "";
  const lower = f.label.toLowerCase();
  const noStore = !storeConnected;
  return (
    <div>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-medium text-fg">{f.label}</p>
          {f.help && <p className="mt-0.5 text-xs text-fg-subtle">{f.help}</p>}
        </div>
        {!edit && !disabled && (
          <div className="flex flex-wrap gap-2">
            {where === "environment" && (
              <Button
                size="sm"
                variant="secondary"
                isDisabled={noStore}
                onPress={() => onEdit({ op: "import" })}
                tooltip={noStore ? "Connect the secret store first" : `Copy the ${lower} from the environment into Vaultix when you save`}
              >
                Move to Vaultix
              </Button>
            )}
            <Button
              size="sm"
              variant="secondary"
              isDisabled={noStore}
              onPress={() => onEdit({ op: "replace", value: "" })}
              tooltip={
                noStore
                  ? "Connect the secret store first"
                  : where
                    ? `Enter a new ${lower}; it replaces the saved one when you save`
                    : `Enter the ${lower}`
              }
            >
              {where ? "Replace" : "Enter key"}
            </Button>
            {where && (
              <Button size="sm" variant="ghost" onPress={() => onEdit({ op: "remove" })} tooltip={`Remove the ${lower} when you save`}>
                Remove
              </Button>
            )}
          </div>
        )}
      </div>
      {edit?.op === "replace" && (
        <div className="mt-3 space-y-2">
          <TextField
            label={`New ${lower}`}
            value={edit.value}
            onChange={(v) => onEdit({ op: "replace", value: v })}
            isDisabled={disabled}
            autoFocus
            type={f.kind === "key" ? "password" : "text"}
            multiline={f.kind === "long_key"}
            mono
            autoComplete="off"
            description="It goes straight to Vaultix and is never shown again."
            isInvalid={!!error}
            aria-describedby={error ? errId : undefined}
          />
          {error && (
            <p id={errId} className="text-xs text-danger">
              {error}
            </p>
          )}
          <Button size="sm" variant="ghost" isDisabled={disabled} onPress={() => onEdit(null)} tooltip="Keep the key as it is">
            Cancel
          </Button>
        </div>
      )}
      {(edit?.op === "remove" || edit?.op === "import") && (
        <div className="mt-3 flex flex-wrap items-center gap-2 text-sm text-fg-muted">
          <Badge tone={edit.op === "remove" ? "warning" : "info"}>
            {edit.op === "remove" ? "Will be removed" : "Will move to Vaultix"}
          </Badge>
          <span>
            {edit.op === "remove"
              ? `The ${lower} is removed when you save.`
              : `The ${lower} is copied from the environment into Vaultix when you save.`}
          </span>
          <Button size="sm" variant="ghost" isDisabled={disabled} onPress={() => onEdit(null)} tooltip="Leave the key as it is">
            Undo
          </Button>
        </div>
      )}
      <Origin view={view} f={f} />
      {!edit && where === "environment" && view.version === 0 && storeConnected && (
        <p className="mt-1.5 text-xs text-fg-subtle">It moves to Vaultix by itself with this section's first save.</p>
      )}
      {noStore && !disabled && (
        <p className="mt-1.5 text-xs text-fg-subtle">
          Keys are saved in Vaultix, and no secret store is connected.{" "}
          <Link href="/admin/secret-store" className="focus-ring rounded-sm text-primary underline-offset-2 data-hovered:underline">
            Connect one on the Secret store page
          </Link>
          .
        </p>
      )}
    </div>
  );
}
