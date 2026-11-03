import { lazy } from "react";
import { Navigate } from "../lib/router/router";
import { type RouteObject } from "../lib/router/match";

// The provider panel's pages, mounted under /admin by the console app and by the operator app
// (ops.<domain>, its own bundle). Each page is its own chunk.
export const AdminLayout = lazy(() => import("../template/admin/admin-layout").then((m) => ({ default: m.AdminLayout })));
const AdminTenants = lazy(() => import("../features/admin/ui/tenants-page").then((m) => ({ default: m.TenantsPage })));
const AdminWorkspaces = lazy(() => import("../features/admin/ui/workspaces-page").then((m) => ({ default: m.AdminWorkspacesPage })));
const AdminIdentity = lazy(() => import("../features/admin/ui/identity-page").then((m) => ({ default: m.IdentityPage })));
const AdminHost = lazy(() => import("../features/admin/ui/host-page").then((m) => ({ default: m.HostPage })));
const AdminEdge = lazy(() => import("../features/admin/ui/edge-page").then((m) => ({ default: m.EdgePage })));
const AdminTemplates = lazy(() => import("../features/admin/ui/templates/templates-page").then((m) => ({ default: m.TemplatesPage })));
const AdminTeam = lazy(() => import("../features/admin/ui/team/team-page").then((m) => ({ default: m.TeamPage })));
const AdminAccess = lazy(() => import("../features/admin/ui/access/access-page").then((m) => ({ default: m.AccessPage })));
const AdminCustomers = lazy(() => import("../features/admin/ui/catalog/customers-page").then((m) => ({ default: m.CustomersPage })));
const AdminCustomer = lazy(() => import("../features/admin/ui/catalog/customer-detail-page").then((m) => ({ default: m.CustomerDetailPage })));
const AdminStaffMode = lazy(() => import("../features/admin/ui/staff-mode/staff-mode-page").then((m) => ({ default: m.StaffModePage })));
const AdminPlatform = lazy(() => import("../features/admin/ui/platform/platform-settings-page").then((m) => ({ default: m.PlatformSettingsPage })));
const AdminInterface = lazy(() => import("../features/admin/ui/interface/interface-page").then((m) => ({ default: m.InterfacePage })));
const AdminPlans = lazy(() => import("../features/admin/ui/catalog/plans-page").then((m) => ({ default: m.PlansPage })));
const AdminServices = lazy(() => import("../features/admin/ui/catalog/services-page").then((m) => ({ default: m.ServicesPage })));

export const adminRoutes: RouteObject[] = [
  { index: true, element: <Navigate to="/admin/tenants" replace /> },
  { path: "tenants", element: <AdminTenants /> },
  { path: "staff-mode", element: <AdminStaffMode /> },
  { path: "workspaces", element: <AdminWorkspaces /> },
  { path: "customers", element: <AdminCustomers /> },
  { path: "customers/:customerId", element: <AdminCustomer /> },
  { path: "plans", element: <AdminPlans /> },
  { path: "services", element: <AdminServices /> },
  { path: "access", element: <AdminAccess /> },
  { path: "team", element: <AdminTeam /> },
  { path: "platform", element: <AdminPlatform /> },
  { path: "interface", element: <AdminInterface /> },
  { path: "templates", element: <AdminTemplates /> },
  { path: "identity", element: <AdminIdentity /> },
  { path: "host", element: <AdminHost /> },
  { path: "edge", element: <AdminEdge /> },
];
