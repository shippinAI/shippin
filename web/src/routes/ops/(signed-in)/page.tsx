import { Navigate } from "../../../lib/router/router";

/** The operator app opens the provider panel's Get started. */
export default function ToStart() {
  return <Navigate to="/admin/start" replace />;
}
