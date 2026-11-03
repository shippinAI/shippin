import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { RouterProvider } from "./lib/router/router";
import { opsRouter } from "./app/ops-router";
import "./styles/app.css";

// Entry of the operator app (ops.html); the customer console's entry is main.tsx.
const queryClient = new QueryClient({ defaultOptions: { queries: { refetchOnWindowFocus: false } } });

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={opsRouter} />
    </QueryClientProvider>
  </StrictMode>,
);
