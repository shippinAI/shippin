import { Navigate } from "../../../lib/router/router";

/** /admin opens Get started. */
export default function AdminHome() {
  return <Navigate to="/admin/start" replace />;
}
