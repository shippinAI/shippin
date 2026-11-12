import { useEffect, useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { usePublicSettings } from "../../../lib/hooks/use-api";
import { useSearchParams } from "../../../lib/router/router";
import { BrandMark } from "../../../template/brand-mark";

const field = "h-10 rounded-lg border border-line-control bg-canvas px-3 text-base";

const post = (path: string, body: unknown) =>
  fetch(path, { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(body) });

/** Public: ask to join the waitlist, then confirm the address from the emailed link or code. */
export function WaitlistPage() {
  const s = usePublicSettings().data;
  const [params] = useSearchParams();
  const linked = params.get("token") ?? "";
  const [stage, setStage] = useState<"ask" | "sent" | "confirmed">("ask");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const confirm = (token: string) => {
    setBusy(true);
    setError("");
    void post("/v1/waitlist/confirm", { token: token.trim() })
      .then((res) => (res.ok ? setStage("confirmed") : setError("That code has been used or has expired.")))
      .finally(() => setBusy(false));
  };
  useEffect(() => {
    if (linked) confirm(linked);
  }, [linked]);

  const open = s?.signup === "waitlist";
  return (
    <div className="flex h-full items-center justify-center p-10">
      <div className="w-full max-w-md">
        <div className="mb-8 flex items-center gap-3">
          <BrandMark size={36} logoURL={s?.brand.logo_url} />
          <span className="text-xl font-semibold tracking-tight">{s?.brand.name}</span>
        </div>
        {stage === "confirmed" ? (
          <div>
            <h1 className="text-3xl font-semibold tracking-tight">You're on the list</h1>
            <p className="mt-2 text-base text-fg-muted">Your address is confirmed. When a place opens, you'll get an invitation by email.</p>
          </div>
        ) : stage === "sent" || linked ? (
          <div>
            <h1 className="text-3xl font-semibold tracking-tight">Check your email</h1>
            <p className="mt-2 text-base text-fg-muted">Open the link we sent, or enter the code from the email here.</p>
            <form className="mt-6 grid gap-2 rounded-xl border border-line bg-surface p-5"
              onSubmit={(e) => { e.preventDefault(); confirm(String(new FormData(e.currentTarget).get("code") ?? "")); }}>
              <label className="text-sm text-fg-muted" htmlFor="waitlist-code">Code</label>
              <input id="waitlist-code" name="code" autoComplete="one-time-code" className={`${field} font-mono`} />
              {error && <p role="alert" className="text-sm text-danger">{error}</p>}
              <Button type="submit" variant="primary" size="lg" className="mt-2 w-full" isDisabled={busy} tooltip="Confirm your address">Confirm</Button>
            </form>
          </div>
        ) : !open ? (
          <div>
            <h1 className="text-3xl font-semibold tracking-tight">The waitlist is closed</h1>
            <p className="mt-2 text-base text-fg-muted">
              {s?.signup === "open" ? <>Anyone can sign up. <a className="font-medium underline" href="/login">Go to sign in</a>.</> : "This installation is invitation only. Ask someone who runs it for an invitation."}
            </p>
          </div>
        ) : (
          <div>
            <h1 className="text-3xl font-semibold tracking-tight">Join the waitlist</h1>
            <p className="mt-2 text-base text-fg-muted">Leave your address. We'll email a link to confirm it, then invite you when a place opens.</p>
            <form className="mt-6 grid gap-2 rounded-xl border border-line bg-surface p-5"
              onSubmit={(e) => {
                e.preventDefault();
                const f = new FormData(e.currentTarget);
                const get = (k: string) => String(f.get(k) ?? "").trim();
                setBusy(true);
                setError("");
                void post("/v1/waitlist", { email: get("email"), name: get("name"), company: get("company"), notes: get("notes") })
                  .then(async (res) => {
                    if (res.ok) return setStage("sent");
                    const p = (await res.json().catch(() => null)) as { message?: string } | null;
                    setError(res.status === 429 ? "Too many tries from here. Wait a few minutes and try again." : (p?.message ?? "That didn't go through. Check the address and try again."));
                  })
                  .finally(() => setBusy(false));
              }}>
              <label className="text-sm text-fg-muted" htmlFor="waitlist-email">Email</label>
              <input id="waitlist-email" name="email" type="email" required autoComplete="email" className={field} />
              <label className="mt-2 text-sm text-fg-muted" htmlFor="waitlist-name">Your name</label>
              <input id="waitlist-name" name="name" autoComplete="name" className={field} />
              <label className="mt-2 text-sm text-fg-muted" htmlFor="waitlist-company">Company (optional)</label>
              <input id="waitlist-company" name="company" autoComplete="organization" className={field} />
              <label className="mt-2 text-sm text-fg-muted" htmlFor="waitlist-notes">What would you like to build? (optional)</label>
              <textarea id="waitlist-notes" name="notes" rows={3} className="rounded-lg border border-line-control bg-canvas px-3 py-2 text-base" />
              {error && <p role="alert" className="text-sm text-danger">{error}</p>}
              <Button type="submit" variant="primary" size="lg" className="mt-2 w-full" isDisabled={busy} tooltip="Ask to join the waitlist">Join the waitlist</Button>
            </form>
          </div>
        )}
      </div>
    </div>
  );
}
