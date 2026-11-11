import { lazy, Suspense } from "react";
import { createBrowserRouter, Navigate } from "../lib/router/router";
import { adminRoutes, AdminLayout } from "./admin-routes";
import { RequireAuth } from "./require-auth";
import { RootLayout } from "./root-layout";

const SignInPage = lazy(() => import("../features/auth/ui/sign-in-page").then((m) => ({ default: m.SignInPage })));

/** The operator app (ops.<domain>): only the provider panel, its own bundle and sign-in. */
export const opsRouter = createBrowserRouter([
  {
    element: <RootLayout />,
    children: [
      { path: "/login", element: <Suspense fallback={null}><SignInPage /></Suspense> },
      {
        element: <RequireAuth />,
        children: [
          { index: true, element: <Navigate to="/admin/start" replace /> },
          { path: "/admin", element: <Suspense fallback={null}><AdminLayout /></Suspense>, children: adminRoutes },
          { path: "*", element: <Navigate to="/admin/start" replace /> },
        ],
      },
    ],
  },
]);
