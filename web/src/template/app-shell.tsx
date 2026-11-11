import { useMe } from "../lib/hooks/use-api";
import { useText } from "../lib/ui-config/use-ui-config";
import { lazy, Suspense } from "react";
import { Outlet, useLocation } from "../lib/router/router";
import { AccessBanner } from "../features/access/ui/access-banner";
import { SignInRequired } from "../features/access/ui/sign-in-required";
import { useTenant } from "../lib/hooks/use-tenant";
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
  [/\/deployments$/, "Services"],
  [/\/workspaces\/new$/, "New workspace"],
  [/\/workspaces\/[^/]+$/, "Workspace"],
  [/\/workspaces$/, "Workspaces"],
  [/^\/ops\/servers$/, "Operations · Servers"],
  [/\/operator\/instances$/, "Instances"],
  [/\/operator$/, "Platform health"],
  [/^\/hub$/, "Agent hub"],
];

export function AppShell() {
  const chatOn = useMe().data?.capabilities["feature.chat"] === true;
  const { pathname } = useLocation();
  const t = useText();
  const { tenant } = useTenant();
  const refused = tenant?.sign_in_met === false ? tenant : undefined;
  const crumb = pathname.startsWith("/operator/hosts") ? t("infra.title") : crumbs.find(([re]) => re.test(pathname))?.[1] ?? "Home";
  return (
    <div className="flex h-full">
      <Sidebar />
      <div className="relative flex min-w-0 flex-1 flex-col overflow-y-auto">
        <Topbar crumb={crumb} />
        <main className={`relative mx-auto w-full ${pathname === "/hub" ? "max-w-none" : "max-w-6xl"} px-4 py-6 sm:px-8 sm:py-9`}>
          {refused ? <SignInRequired tenant={refused} /> : <AccessBanner />}
          <Suspense fallback={null}>
            {!refused && <Outlet />}
            {chatOn && <Suspense fallback={null}><ChatLauncher /></Suspense>}
          </Suspense>
        </main>
      </div>
    </div>
  );
}
