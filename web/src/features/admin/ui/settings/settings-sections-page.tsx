import { Link } from "react-aria-components";
import { LinkButton } from "../../../../lib/design/components/button/link-button";
import { Notice } from "../../../../lib/design/components/notice/notice";
import { Skeleton } from "../../../../lib/design/components/skeleton/skeleton";
import { useParams } from "../../../../lib/router/router";
import { PageHeader } from "../../../../template/page-header";
import { problemOf, sentence } from "./model";
import { SectionForm } from "./section-form";
import { SectionHistory } from "./section-history";
import { useCanWriteSettings, useSecretStore, useSettingSection, useSettingSections } from "./use-settings";

/** Settings > Sections (CFG-01, docs/settings.md): every configuration section as a form generated from
 * its own description. The section is in the URL, /admin/settings/:section. */
export function SettingsSectionsPage() {
  const { section } = useParams();
  const sections = useSettingSections();
  const store = useSecretStore();
  const canWrite = useCanWriteSettings();
  const items = sections.data?.items ?? [];
  const current = items.find((s) => s.key === section) ?? items[0];
  const storeConnected = !!store.data?.connected;
  return (
    <>
      <PageHeader
        title="Settings"
        description="How this installation runs, section by section. A change takes effect without a restart, needs a reason, and is kept in the section's history."
        actions={
          <>
            <LinkButton href="/admin/secret-store" variant="secondary" size="sm" tooltip="Where the keys entered here are kept">
              Secret store
            </LinkButton>
            <LinkButton
              href="/admin/platform"
              variant="ghost"
              size="sm"
              tooltip="Older overrides of single config keys, until they move here"
            >
              Platform settings
            </LinkButton>
          </>
        }
      />
      <div className="mb-6 space-y-3">
        {store.data && !store.data.connected && (
          <Notice
            tone="warning"
            title="No secret store is connected"
            actions={
              <LinkButton href="/admin/secret-store" size="sm" variant="secondary" tooltip="Connect this installation to Vaultix">
                Connect Vaultix
              </LinkButton>
            }
          >
            Keys, such as private keys and webhook secrets, are saved in Vaultix. Until it is connected you can change everything else here,
            but not keys.
          </Notice>
        )}
        {store.data?.connected && store.data.check_error && (
          <Notice
            tone="danger"
            title="The last check of the secret store failed"
            actions={
              <LinkButton href="/admin/secret-store" size="sm" variant="secondary" tooltip="See the connection and check it again">
                Open Secret store
              </LinkButton>
            }
          >
            {sentence(store.data.check_error)} Saving keys may fail until it is fixed.
          </Notice>
        )}
        {store.isError && (
          <Notice tone="danger" title="Couldn't read the secret store">
            {problemOf(store.error).message}
          </Notice>
        )}
        {sections.isError && (
          <Notice tone="danger" title="Couldn't load the settings">
            {problemOf(sections.error).message}
          </Notice>
        )}
      </div>
      {sections.isPending && <Skeleton className="h-64 w-full" />}
      {sections.data && items.length === 0 && <p className="text-base text-fg-muted">This installation has no settings sections.</p>}
      {current && (
        <div className="grid gap-6 lg:grid-cols-[minmax(0,15rem)_minmax(0,1fr)]">
          <nav aria-label="Settings sections">
            <ul className="space-y-1">
              {items.map((s) => {
                const on = s.key === current.key;
                return (
                  <li key={s.key}>
                    <Link
                      href={`/admin/settings/${encodeURIComponent(s.key)}`}
                      aria-current={on ? "page" : undefined}
                      className={`focus-ring block rounded-lg border px-3 py-2 ${on ? "border-line-strong bg-raised text-fg" : "border-transparent text-fg-muted data-hovered:bg-raised/60 data-hovered:text-fg"}`}
                    >
                      <span className="block text-sm font-medium">{s.title}</span>
                      <span className="block text-xs text-fg-subtle">
                        {s.version > 0 ? "Saved in the panel" : "From the bootstrap file"}
                      </span>
                    </Link>
                  </li>
                );
              })}
            </ul>
          </nav>
          <Section key={current.key} settingId={current.key} storeConnected={storeConnected} canWrite={canWrite} />
        </div>
      )}
    </>
  );
}

function Section({ settingId, storeConnected, canWrite }: { settingId: string; storeConnected: boolean; canWrite: boolean }) {
  const q = useSettingSection(settingId);
  if (q.isError)
    return (
      <Notice tone="danger" title="Couldn't load this section">
        {problemOf(q.error).message}
      </Notice>
    );
  if (!q.data) return <Skeleton className="h-64 w-full" />;
  const view = q.data;
  return (
    <div className="min-w-0 space-y-6">
      {/* A new version (a save, or a reload after someone else's) starts the form again from what is saved. */}
      <SectionForm key={view.version} view={view} storeConnected={storeConnected} canWrite={canWrite} onReload={() => void q.refetch()} />
      <SectionHistory view={view} />
    </div>
  );
}
