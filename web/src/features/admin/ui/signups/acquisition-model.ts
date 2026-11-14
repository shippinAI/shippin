import type { AcquisitionSourceDetail, AcquisitionSourceSummary } from '../../../../lib/api/actions.gen';

export type AcquisitionCapabilities = {
  waitlist: boolean;
  platformRead: boolean;
  platformWrite: boolean;
  sourceRead: boolean;
  sourceConfigure: boolean;
  sourceSync: boolean;
  enterSignups: boolean;
};

export function acquisitionCapabilities(permissions: readonly string[]): AcquisitionCapabilities {
  const has = (permission: string) => permissions.includes(permission);
  const waitlist = has('customers.read');
  const platformRead = has('platform.settings.read');
  const platformWrite = has('platform.settings.write');
  const sourceRead = has('acquisition.read');
  const sourceSync = has('acquisition.sync');
  return {
    waitlist,
    platformRead,
    platformWrite,
    sourceRead,
    sourceConfigure: sourceRead && platformWrite,
    sourceSync,
    enterSignups: waitlist || platformRead || platformWrite || sourceRead,
  };
}

export type AcquisitionSourceState = 'paused' | 'never-synced' | 'synced';

export function acquisitionSourceState(source: AcquisitionSourceSummary): AcquisitionSourceState {
  if (source.paused) return 'paused';
  return source.latest_success_at ? 'synced' : 'never-synced';
}

export function validSourceOriginPort(origin: string, portText: string): boolean {
  if (!/^\d{1,5}$/.test(portText)) return false;
  const port = Number(portText);
  if (!Number.isInteger(port) || port < 1 || port > 65535) return false;
  try {
    const parsed = new URL(origin);
    if (parsed.protocol !== 'https:' || !parsed.hostname || parsed.username || parsed.password ||
      parsed.pathname !== '/' || parsed.search || parsed.hash) return false;
    return parsed.port === '' || Number(parsed.port) === port;
  } catch {
    return false;
  }
}

export function leadPreviewState(total: number, visible: number, truncated: boolean): 'empty' | 'latest' | 'complete' {
  if (visible === 0) return 'empty';
  return truncated || total > visible ? 'latest' : 'complete';
}

export function sourceDraftIsStale(loadedGeneration: number | null, liveGeneration: number): boolean {
  return loadedGeneration === null || loadedGeneration !== liveGeneration;
}

/** Pause/resume changes only the state flag, using the persisted config, never unsaved form values. */
export function persistedSourceStateUpdate(detail: AcquisitionSourceDetail, paused: boolean) {
  return {
    expected_config_generation: detail.summary.config_generation,
    label: detail.summary.label,
    origin: detail.origin,
    dial_ip: detail.dial_ip,
    port: detail.port,
    ca_pem: detail.ca_pem,
    paused,
  };
}
