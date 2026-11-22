import { Navigate } from "../../../../lib/router/router";

/** Anything else in the operator app opens the provider panel's Get started. */
export default function ToStart() {
  return <Navigate to="/admin/start" replace />;
}
