import { lazy, Suspense } from "react";
import { createBrowserRouter } from "../lib/router/router";
import { RootLayout } from "./root-layout";
import { LandingRedirect, RequireAuth } from "./require-auth";

// Everything but the root layout and the auth gate is its own chunk (initial JS budget, CONSOLE-IA).
const SignInPage = lazy(() => import("../features/auth/ui/sign-in-page").then((m) => ({ default: m.SignInPage })));
const InvitePage = lazy(() => import("../features/auth/ui/invite-page").then((m) => ({ default: m.InvitePage })));
const SetupPage = lazy(() => import("../features/auth/ui/setup-page").then((m) => ({ default: m.SetupPage })));
const GoPage = lazy(() => import("../features/go/ui/go-page").then((m) => ({ default: m.GoPage })));
const OverviewPage = lazy(() => import("../features/overview/ui/overview-page").then((m) => ({ default: m.OverviewPage })));
const AppShell = lazy(() => import("../template/app-shell").then((m) => ({ default: m.AppShell })));
const AdminOrigin = lazy(() => import("../template/admin/admin-origin").then((m) => ({ default: m.AdminOrigin })));
const InstancesPage = lazy(() => import("../features/instances/ui/instances-page").then((m) => ({ default: m.InstancesPage })));
const InfrastructurePage = lazy(() => import("../features/infrastructure/ui/infrastructure-page").then((m) => ({ default: m.InfrastructurePage })));
const PlatformPage = lazy(() => import("../features/instances/ui/platform-page").then((m) => ({ default: m.PlatformPage })));
const SettingsPage = lazy(() => import("../features/settings/ui/settings-page").then((m) => ({ default: m.SettingsPage })));
const WorkspacesPage = lazy(() => import("../features/workspaces/ui/workspaces-page").then((m) => ({ default: m.WorkspacesPage })));
const WorkspaceDetailPage = lazy(() => import("../features/workspaces/ui/workspace-detail-page").then((m) => ({ default: m.WorkspaceDetailPage })));
const AIConnectionsPage = lazy(() => import("../features/account/ui/ai-connections-page").then((m) => ({ default: m.AIConnectionsPage })));
const SecurityPage = lazy(() => import("../features/account/ui/security-page").then((m) => ({ default: m.SecurityPage })));
const SSHKeysPage = lazy(() => import("../features/account/ui/ssh-keys-page").then((m) => ({ default: m.SSHKeysPage })));
const CreateWorkspacePage = lazy(() => import("../features/workspaces/ui/create/create-workspace-page").then((m) => ({ default: m.CreateWorkspacePage })));
const ProjectsPage = lazy(() => import("../features/projects/ui/projects-page").then((m) => ({ default: m.ProjectsPage })));
const DevspacePage = lazy(() => import("../features/projects/ui/devspace-page").then((m) => ({ default: m.DevspacePage })));
const OpsServersPage = lazy(() => import("../features/orgs/ui/ops-servers-page").then((m) => ({ default: m.OpsServersPage })));
const ServersPage = lazy(() => import("../features/servers/ui/servers-page").then((m) => ({ default: m.ServersPage })));
const ServerGuestsPage = lazy(() => import("../features/servers/ui/server-guests-page").then((m) => ({ default: m.ServerGuestsPage })));
const IdentityPage = lazy(() => import("../features/identity/ui/identity-page").then((m) => ({ default: m.IdentityPage })));
const BridgeSessionsPage = lazy(() => import("../features/account/ui/bridge-sessions-page").then((m) => ({ default: m.BridgeSessionsPage })));
const ChatPage = lazy(() => import("../features/chat/ui/chat-page").then((m) => ({ default: m.ChatPage })));
const DevicePage = lazy(() => import("../features/bridge/ui/device-page").then((m) => ({ default: m.DevicePage })));
const ConnectAccountsPage = lazy(() => import("../features/connect/ui/connect-accounts-page").then((m) => ({ default: m.ConnectAccountsPage })));
const TerminalPage = lazy(() => import("../features/terminal/ui/terminal-page").then((m) => ({ default: m.TerminalPage })));
const GoldPendingPage = lazy(() => import("../features/gold-sign/ui/pending-page").then((m) => ({ default: m.PendingPage })));
const GoldTicketPage = lazy(() => import("../features/gold-sign/ui/ticket-page").then((m) => ({ default: m.TicketPage })));
const GoldPasskeysPage = lazy(() => import("../features/gold-sign/ui/passkeys-page").then((m) => ({ default: m.PasskeysPage })));
const HubPage = lazy(() => import("../features/hub/ui/hub-page").then((m) => ({ default: m.HubPage })));
const VaultixPage = lazy(() => import("../features/vaultix/ui/vaultix-page").then((m) => ({ default: m.VaultixPage })));
const TeamPage = lazy(() => import("../features/team/ui/team-page").then((m) => ({ default: m.TeamPage })));
const DesktopPage = lazy(() => import("../features/apps/ui/desktop-page").then((m) => ({ default: m.DesktopPage })));
const OnboardingPage = lazy(() => import("../features/onboarding/ui/onboarding-page").then((m) => ({ default: m.OnboardingPage })));
const DatabasePage = lazy(() => import("../features/database/ui/database-page").then((m) => ({ default: m.DatabasePage })));
export const router = createBrowserRouter([
  {
    element: <RootLayout />,
    children: [
      { path: "/login", element: <Suspense fallback={null}><SignInPage /></Suspense> },
      { path: "/setup", element: <Suspense fallback={null}><SetupPage /></Suspense> },
      { path: "/invite", element: <Suspense fallback={null}><InvitePage /></Suspense> },
      { path: "/device", element: <Suspense fallback={null}><DevicePage /></Suspense> },
      {
        element: <RequireAuth />,
        children: [
          { index: true, element: <LandingRedirect /> },
          { path: "/admin/*", element: <Suspense fallback={null}><AdminOrigin /></Suspense> },
          { path: "/welcome", element: <Suspense fallback={null}><OnboardingPage /></Suspense> },
          {
            element: <Suspense fallback={null}><AppShell /></Suspense>,
            children: [
              { path: "/t/:tenantId", element: <Suspense fallback={null}><OverviewPage /></Suspense> },
              { path: "/t/:tenantId/settings", element: <SettingsPage /> },
              { path: "/t/:tenantId/go/:feature", element: <Suspense fallback={null}><GoPage /></Suspense> },
              { path: "/t/:tenantId/projects", element: <ProjectsPage /> },
              { path: "/t/:tenantId/database", element: <Suspense fallback={null}><DatabasePage /></Suspense> },
              { path: "/t/:tenantId/projects/:projectId", element: <DevspacePage /> },
              { path: "/t/:tenantId/workspaces", element: <WorkspacesPage /> },
              { path: "/t/:tenantId/identity", element: <IdentityPage /> },
              { path: "/t/:tenantId/servers", element: <ServersPage /> },
              { path: "/t/:tenantId/servers/:serverId", element: <Suspense fallback={null}><ServerGuestsPage /></Suspense> },
              { path: "/ops/servers", element: <OpsServersPage /> },
              { path: "/t/:tenantId/team", element: <TeamPage /> },
              { path: "/t/:tenantId/workspaces/new", element: <CreateWorkspacePage /> },
              { path: "/t/:tenantId/workspaces/:workspaceId", element: <WorkspaceDetailPage /> },
              { path: "/t/:tenantId/workspaces/:workspaceId/connect", element: <Suspense fallback={null}><ConnectAccountsPage /></Suspense> },
              { path: "/t/:tenantId/workspaces/:workspaceId/terminal", element: <Suspense fallback={null}><TerminalPage /></Suspense> },
              { path: "/t/:tenantId/desktop", element: <Suspense fallback={null}><DesktopPage /></Suspense> },
              { path: "/account/security", element: <Suspense fallback={null}><SecurityPage /></Suspense> },
              { path: "/account/ssh-keys", element: <SSHKeysPage /> },
              { path: "/account/ai", element: <AIConnectionsPage /> },
              { path: "/account/bridge", element: <BridgeSessionsPage /> },
              { path: "/hub", element: <Suspense fallback={null}><HubPage /></Suspense> },
              { path: "/vaultix", element: <Suspense fallback={null}><VaultixPage /></Suspense> },
              { path: "/gold", element: <Suspense fallback={null}><GoldPendingPage /></Suspense> },
              { path: "/gold/passkeys", element: <Suspense fallback={null}><GoldPasskeysPage /></Suspense> },
              { path: "/gold/:ticketId", element: <Suspense fallback={null}><GoldTicketPage /></Suspense> },
              { path: "/chat", element: <ChatPage /> },
              { path: "/chat/:sessionId", element: <ChatPage /> },
              { path: "/operator", element: <PlatformPage /> },
              { path: "/operator/instances", element: <InstancesPage /> },
              { path: "/operator/hosts", element: <Suspense fallback={null}><InfrastructurePage /></Suspense> },
              { path: "/operator/hosts/:tab", element: <Suspense fallback={null}><InfrastructurePage /></Suspense> },
            ],
          },
        ],
      },
    ],
  },
]);
