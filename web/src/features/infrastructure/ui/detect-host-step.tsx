import { useState, type FormEvent } from "react";
import type { HvCertificateInfo, HvDetection } from "../../../lib/api/actions.gen";
import { Button } from "../../../lib/design/components/button/button";
import { TextField } from "../../../lib/design/components/field/text-field";
import { Notice } from "../../../lib/design/components/notice/notice";
import { useDetectHost, type InfraProvider } from "../../../lib/hooks/use-infra";
import { useText } from "../../../lib/ui-config/use-ui-config";
import { prefillFromDetection, type DetectionPrefill } from "../model";

/** A certificate's fingerprint, names and expiry: what a person compares before trusting it. */
export function CertificateFacts({ cert }: { cert: Pick<HvCertificateInfo, "fingerprint" | "names" | "not_after"> }) {
  const t = useText();
  return (
    <dl className="space-y-1 text-fg-muted">
      <div>
        <dt className="inline font-medium text-fg">{t("infra.trust.fingerprint")} </dt>
        <dd className="inline break-all font-mono text-xs" data-testid="fingerprint">
          {cert.fingerprint}
        </dd>
      </div>
      <div>
        <dt className="inline font-medium text-fg">{t("infra.trust.names")} </dt>
        <dd className="inline">{cert.names.join(", ")}</dd>
      </div>
      <div>
        <dt className="inline font-medium text-fg">{t("infra.trust.expires")} </dt>
        <dd className="inline">{new Date(cert.not_after).toLocaleDateString()}</dd>
      </div>
    </dl>
  );
}

type Props = {
  providers: InfraProvider[];
  onConfirm: (prefill: DetectionPrefill, found: HvDetection) => void;
  onChooseKind: () => void;
};

/** The first step of adding a host: the address alone, what answers there, and the person's confirmation
 * before any credential is asked for. Nothing is sent to the server but an unauthenticated request. */
export function DetectHostStep({ providers, onConfirm, onChooseKind }: Props) {
  const t = useText();
  const detect = useDetectHost();
  const [address, setAddress] = useState("");
  const found = detect.data;
  const prefill = prefillFromDetection(found, providers);
  const label = (kind?: string) => providers.find((p) => p.kind === kind)?.label ?? kind ?? "";
  // What the whole detection adds to its probes, such as a second server found on another port.
  const extra = found ? found.evidence.filter((e) => !found.probes.some((p) => p.evidence.includes(e))) : [];
  const run = (e?: FormEvent) => {
    e?.preventDefault();
    if (address.trim()) detect.mutate(address.trim());
  };
  const manual = (
    <Button variant={prefill ? "ghost" : "secondary"} tooltip={t("infra.detect.manualTooltip")} onPress={onChooseKind}>
      {t("infra.detect.manual")}
    </Button>
  );
  return (
    <form className="space-y-4" onSubmit={run} data-testid="detect-step">
      <TextField
        label={t("infra.detect.address")}
        description={t("infra.detect.address.help")}
        value={address}
        mono
        autoFocus
        autoComplete="off"
        onChange={(v) => {
          setAddress(v);
          detect.reset();
        }}
      />
      <div className="flex flex-wrap justify-end gap-2">
        {!found && manual}
        <Button type="submit" variant="primary" isDisabled={!address.trim() || detect.isPending} tooltip={t("infra.detect.runTooltip")}>
          {detect.isPending ? t("infra.detect.running") : t("infra.detect.run")}
        </Button>
      </div>
      {detect.isError && (
        <Notice tone="danger" title={t("infra.detect.failed")}>
          {detect.error.message}
        </Notice>
      )}
      {found && (
        <Notice
          tone={prefill ? (found.certificate_verified ? "success" : "warning") : found.outcome === "unreachable" ? "danger" : "info"}
          title={
            prefill
              ? t("infra.detect.found", { provider: label(found.kind), url: found.base_url })
              : found.outcome === "recognised"
                ? t("infra.detect.notOffered", { provider: label(found.kind), url: found.base_url })
                : found.outcome === "unrecognised"
                  ? t("infra.detect.unrecognised", { address: found.address })
                  : t("infra.detect.unreachable", { address: found.address })
          }
          actions={
            <>
              {manual}
              {prefill && (
                <Button variant="primary" tooltip={t("infra.detect.confirmTooltip")} onPress={() => onConfirm(prefill, found)}>
                  {t("infra.detect.confirm")}
                </Button>
              )}
            </>
          }
        >
          <div className="space-y-2" data-testid="detect-result" data-outcome={found.outcome}>
            {found.version && <p>{t("infra.detect.version", { version: found.version })}</p>}
            {found.certificate && (
              <>
                <p>{found.certificate_verified ? t("infra.detect.certVerified") : t("infra.detect.certCompare")}</p>
                <CertificateFacts cert={found.certificate} />
              </>
            )}
            {found.outcome === "unreachable" && <p>{t("infra.detect.unreachable.help")}</p>}
            <p className="font-medium text-fg">{t("infra.detect.evidence")}</p>
            <ul className="list-disc space-y-0.5 pl-5">
              {found.probes.flatMap((p) =>
                p.evidence.map((e, i) => (
                  <li key={`${p.base_url}:${i}`}>
                    <span className="font-mono text-xs">{p.base_url}</span> {e}
                  </li>
                )),
              )}
              {extra.map((e) => (
                <li key={e}>{e}</li>
              ))}
            </ul>
          </div>
        </Notice>
      )}
    </form>
  );
}
