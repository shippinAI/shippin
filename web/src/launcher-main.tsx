import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { ToastRegionHost } from "./lib/design/components/toast/toast-region";
import { LauncherPage } from "./features/ai-launcher/ui/launcher-page";
import "./styles/app.css";

// Entry of the in-devspace AI launcher (launcher.html), served by cmd/ai-launcher behind the
// workspace's forward_auth; the console and operator apps have their own entries.
const queryClient = new QueryClient({ defaultOptions: { queries: { refetchOnWindowFocus: false } } });

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <LauncherPage />
      <ToastRegionHost />
    </QueryClientProvider>
  </StrictMode>,
);
