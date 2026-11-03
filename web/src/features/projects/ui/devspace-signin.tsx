import { Button } from "../../../lib/design/components/button/button";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";

type Props = { onSignIn: () => void; checking: boolean };

/** Shown instead of the IDE frame until the workspace host has a valid sign-in cookie. */
export function DevspaceSignIn({ onSignIn, checking }: Props) {
  if (checking) {
    return <div className="p-8"><EmptyState title="Connecting to your devspace" description="Checking your sign-in…" /></div>;
  }
  return (
    <div className="p-8">
      <EmptyState
        title="Sign in to open your devspace"
        description="For your security the editor signs in in a small window. It closes by itself when you're done."
        action={
          <Button variant="primary" tooltip="Opens a sign-in window for this devspace" onPress={onSignIn}>
            Sign in
          </Button>
        }
      />
    </div>
  );
}
