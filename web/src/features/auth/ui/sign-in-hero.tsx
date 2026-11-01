import { Boxes, Fingerprint, KeyRound } from "lucide-react";

const tiles = [
  { icon: Boxes, label: "Workspaces", sub: "Incus · ZFS", hue: "var(--color-hue-workspaces)", pos: "top-[22%] left-[14%]" },
  { icon: Fingerprint, label: "Identity", sub: "OIDC · orgs", hue: "var(--color-hue-identity)", pos: "top-[44%] left-[46%]" },
  { icon: KeyRound, label: "Secrets", sub: "per-env vaults", hue: "var(--color-hue-secrets)", pos: "top-[66%] left-[20%]" },
];

/** Flat-vector hero: layered module cards on a grid with tinted glows. */
export function SignInHero({ tagline }: { tagline: string }) {
  return (
    <div className="relative hidden overflow-hidden border-r border-line bg-surface lg:block">
      <div className="grid-bg absolute inset-0" />
      <div className="absolute -top-40 -left-32 size-[520px] rounded-full bg-primary-strong/25 blur-3xl" />
      <div className="absolute -right-24 bottom-[-120px] size-[440px] rounded-full bg-hue-identity/20 blur-3xl" />
      <svg className="absolute inset-0 size-full" aria-hidden>
        <path d="M180 230 C 320 260, 360 330, 420 390" stroke="var(--color-line-strong)" strokeDasharray="4 6" fill="none" />
        <path d="M420 430 C 360 500, 300 520, 230 560" stroke="var(--color-line-strong)" strokeDasharray="4 6" fill="none" />
      </svg>
      {tiles.map(({ icon: Icon, label, sub, hue, pos }) => (
        <div
          key={label}
          className={`absolute ${pos} flex w-56 items-center gap-3 rounded-2xl border border-line-strong bg-raised/90 p-4 shadow-[var(--shadow-e3)]`}
        >
          <span className="flex size-10 items-center justify-center rounded-xl" style={{ background: `color-mix(in oklch, ${hue} 18%, transparent)` }}>
            <Icon className="size-5" style={{ color: hue }} strokeWidth={1.75} />
          </span>
          <div>
            <div className="text-sm font-semibold">{label}</div>
            <div className="text-xs text-fg-subtle">{sub}</div>
          </div>
          <span className="ml-auto size-2 rounded-full bg-success shadow-[0_0_10px] shadow-success" />
        </div>
      ))}
      <div className="absolute right-12 bottom-12 left-12">
        <p className="max-w-md text-2xl leading-snug font-semibold tracking-tight text-fg">{tagline}</p>
        <p className="mt-3 text-sm text-fg-muted">One console for every tenant: workspaces, identity, secrets and the platform beneath.</p>
      </div>
    </div>
  );
}
