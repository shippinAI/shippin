import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { ApiError } from "./lib/api/client";
import { RouterProvider } from "./lib/router/router";
import { router } from "./app/router";
import "./styles/app.css";

// A refusal or a missing thing will not change on retry; only server and network failures are retried.
const queryClient = new QueryClient({ defaultOptions: { queries: { refetchOnWindowFocus: false, retry: (n, e) => n < 3 && !(e instanceof ApiError && e.status < 500) } } });

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
);
