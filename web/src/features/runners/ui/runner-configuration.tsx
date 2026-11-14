import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ApiError } from "../../../lib/api/client";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { Select } from "../../../lib/design/components/select/select";
import { TextField } from "../../../lib/design/components/field/text-field";
import { Button } from "../../../lib/design/components/button/button";
import { errorKey, type RunnerProject } from "../model";
import { grantedProfiles, loadProfile, draftOf, profilePayload, saveProfile, type ProfileSchema, type ProfileTemplate } from "../profiles";

export function ProfileForm({ tenant, project, profile, schema, template, reload }: { tenant: string; project: RunnerProject; profile: string; schema: ProfileSchema; template: ProfileTemplate | null; reload: () => void }) {
  const t = useText();
  const [draft, setDraft] = useState(() => draftOf(template));
  const [revision, setRevision] = useState(template?.revision ?? 0);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<"saved" | "conflict" | "failed" | null>(null);
  const writable = project.scopes.includes("profiles:write");
  const validation = profilePayload(schema, draft);
  const blocked = saving || !writable || message === "conflict" || message === "failed";
  async function save() {
    setSaving(true);
    try {
      const saved = await saveProfile(tenant, project, profile, revision, schema, draft);
      setRevision(saved.revision); setMessage("saved");
    } catch (error) { setMessage(error instanceof ApiError && error.status === 409 ? "conflict" : "failed"); }
    finally { setSaving(false); }
  }
  return <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); if (!blocked && !Object.keys(validation.errors).length) void save(); }}>
    <p>{t("runners.profile.revision", { revision })}</p>
    <p className="text-fg-muted">{t("runners.profile.unverified")}</p>
    {!writable && <p>{t("runners.profile.readOnly")}</p>}
    {message && <p role={message === "saved" ? "status" : "alert"}>{t(`runners.profile.${message}`)}</p>}
    <div className="grid gap-4 md:grid-cols-2">{Object.entries(schema.properties).map(([key, field]) => <TextField
      key={key} label={field.description} type={field.type === "integer" ? "number" : "text"}
      min={field.minimum} max={field.maximum} step={field.type === "integer" ? 1 : undefined}
      value={draft[key] ?? ""} onChange={(value) => { setDraft((d) => ({ ...d, [key]: value })); if (message === "saved") setMessage(null); }}
      isReadOnly={!writable || saving}
      isRequired={schema.required.includes(key)}
      isInvalid={!!validation.errors[key]}
      description={field.default !== undefined ? t("runners.profile.default", { value: field.default }) : t("runners.profile.required")}
      placeholder={field.default === undefined ? undefined : String(field.default)}
    />)}</div>
    {Object.keys(validation.errors).length > 0 && <p role="status">{t("runners.profile.invalid")}</p>}
    <div className="flex flex-wrap gap-3">
      {writable && <Button type="submit" tooltip={t("runners.profile.saveHint")} isDisabled={blocked || !!Object.keys(validation.errors).length}>{t("runners.profile.save")}</Button>}
      <Button tooltip={t("runners.profile.reloadHint")} isDisabled={saving} onPress={reload}>{t("runners.profile.reload")}</Button>
    </div>
  </form>;
}
function SelectedProfile({ tenant, project, profile }: { tenant: string; project: RunnerProject; profile: string }) {
  const t = useText();
  const q = useQuery({ queryKey: ["runner-profile", tenant, project.id, profile, project.scopes], queryFn: () => loadProfile(tenant, project, profile), retry: false, gcTime: 0, staleTime: Infinity, refetchOnWindowFocus: false, refetchOnReconnect: false });
  return <div className="space-y-4">
    {q.isPending && <p role="status">{t("runners.loading")}</p>}
    {q.isError && <p role="alert">{t(errorKey(q.error))}</p>}
    {q.data && <ProfileForm key={q.dataUpdatedAt} tenant={tenant} project={project} profile={profile} schema={q.data.schema} template={q.data.template} reload={() => { void q.refetch(); }} />}
    {!q.data && q.isError && <Button tooltip={t("runners.refresh")} onPress={() => { void q.refetch(); }}>{t("runners.refresh")}</Button>}
  </div>;
}
export function RunnerConfiguration({ tenant, project }: { tenant: string; project: RunnerProject }) {
  const t = useText();
  const profiles = grantedProfiles(project);
  const [selected, select] = useState("");
  const profile = profiles.includes(selected) ? selected : profiles[0];
  return <div className="space-y-5">
    <h2 className="text-lg font-semibold">{t("runners.profile.title")}</h2>
    {!profiles.length ? <p>{t("runners.profile.noGrants")}</p> : <>
      <Select label={t("runners.profile.select")} value={profile} options={profiles.map((id) => ({ id, label: id }))} onChange={select} />
      <SelectedProfile key={profile} tenant={tenant} project={project} profile={profile} />
    </>}
    <h2 className="text-lg font-semibold">{t("runners.profile.credentials")}</h2>
    <p>{t("runners.profile.rotation")}</p>
  </div>;
}
