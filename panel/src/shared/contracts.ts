export type AppMode = "customer" | "operator";

export type RuntimeConfig = {
  appName: string;
  environment: string;
  apiBaseUrl: string;
  docsUrl: string;
  defaultTenantId: string;
  appMode: "customer" | "operator";
  capabilityHints: Record<string, string>;
  auth: {
    enabled: boolean;
    issuerUrl: string;
    clientId: string;
    audience: string;
    scopes: string;
    redirectUri: string;
    sessionExpiryWarningSeconds: number;
  };
};

export type OpenApiOperation = {
  method: string;
  path: string;
  operationId: string;
  summary: string;
  tags: string[];
  implementationStatus: string;
};

export type OpenApiIndex = {
  generatedAt: string;
  sourceFile: string;
  operationCount: number;
  tags: string[];
  operations: OpenApiOperation[];
};

export type ApiExecutionResult = {
  request: {
    method: string;
    url: string;
    headers: Record<string, string>;
    body: string | null;
  };
  response: {
    status: number;
    statusText: string;
    headers: Record<string, string>;
    body: string;
  };
};

export type MeTenant = {
  tenant_id: string;
  name?: string;
  roles?: string[];
};

export type MeIdentity = {
  user_id: string;
  display_name?: string;
  platform_roles?: string[];
  tenants: MeTenant[];
  capabilities?: Record<string, boolean>;
};
