import { useText } from "../lib/ui-config/use-ui-config";
import { lazy, Suspense } from "react";
import { Outlet, useLocation } from "../lib/router/router";
import { AccessBanner } from "../features/access/ui/access-banner";
import { Sidebar } from "./sidebar";
import { Topbar } from "./topbar";

const ChatLauncher = lazy(() => import("../features/chat/ui/chat-launcher").then((m) => ({ default: m.ChatLauncher })));

const crumbs: [RegExp, string][] = [
  [/^\/chat(\/[^/]+)?$/, "Automaton chat"],
  [/\/team$/, "Team"],
  [/\/settings$/, "Settings"],
  [/\/projects\/[^/]+$/, "Devspace"],
  [/\/projects$/, "Projects"],
  [/\/database$/, "Database"],
  [/\/workspaces\/new$/, "New workspace"],
  [/\/workspaces\/[^/]+$/, "Workspace"],
  [/\/workspaces$/, "Workspaces"],
  [/^\/ops\/servers$/, "Operations · Servers"],
  [/\/operator\/instances$/, "Instances"],
  [/\/operator$/, "Platform health"],
  [/^\/hub$/, "Agent hub"],
  [/^\/gold\/passkeys$/, "Passkeys"],
  [/^\/gold/, "Gold approvals"],
];

export function AppShell() {
  const { pathname } = useLocation();
  const t = useText();
  const crumb = pathname.startsWith("/operator/hosts") ? t("infra.title") : crumbs.find(([re]) => re.test(pathname))?.[1] ?? "Home";
  return (
    <div className="flex h-full">
      <Sidebar />
      <div className="relative flex min-w-0 flex-1 flex-col overflow-y-auto">
        <div className="pointer-events-none absolute inset-x-0 top-0 h-72 bg-[radial-gradient(60%_100%_at_30%_0%,oklch(0.66_0.16_250/0.12),transparent)]" />
        <Topbar crumb={crumb} />
        <main className={`relative mx-auto w-full ${pathname === "/hub" ? "max-w-none" : "max-w-6xl"} px-4 py-6 sm:px-8 sm:py-9`}>
          <AccessBanner />
          <Suspense fallback={null}>
            <Outlet />
            {!pathname.startsWith("/gold") && <Suspense fallback={null}><ChatLauncher /></Suspense>}
          </Suspense>
        </main>
      </div>
    </div>
  );
}
