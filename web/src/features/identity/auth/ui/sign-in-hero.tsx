import { Boxes, Fingerprint, Server } from "lucide-react";

const modules = [
  { icon: Server, label: "Infrastructure", sub: "Incus and Proxmox hosts, their guests and capacity", hue: "text-hue-network" },
  { icon: Boxes, label: "Tenants", sub: "Organizations, their workspaces and projects", hue: "text-hue-workspaces" },
  { icon: Fingerprint, label: "Access", sub: "Accounts, passkeys, roles and OIDC sign-in", hue: "text-hue-identity" },
];

/** Beside the sign-in form on wide screens: what this panel manages, then the installation's tagline. */
export function SignInHero({ tagline }: { tagline: string }) {
  return (
    <div className="hidden flex-col justify-between gap-12 border-r border-line bg-surface p-12 lg:flex">
      <ul className="grid max-w-md gap-3" aria-label="What this panel manages">
        {modules.map(({ icon: Icon, label, sub, hue }) => (
          <li key={label} className="flex items-start gap-4 rounded-xl border border-line bg-raised p-4">
            <Icon className={`mt-0.5 size-5 shrink-0 ${hue}`} strokeWidth={1.75} aria-hidden />
            <div>
              <p className="text-base font-semibold text-fg">{label}</p>
              <p className="mt-0.5 text-sm text-fg-muted">{sub}</p>
            </div>
          </li>
        ))}
      </ul>
      <div className="max-w-md">
        <p className="text-2xl leading-snug font-semibold tracking-tight text-fg">{tagline}</p>
        <p className="mt-3 text-base text-fg-muted">One panel for your servers, the tenants that use them, and who can reach them.</p>
      </div>
    </div>
  );
}
