// Local-run mock of the console API for R-J1a screenshots (fixture data, not Gold).
import http from "node:http";
const T = "01aaaaaa-0000-7000-8000-000000000001", W = "01aaaaaa-0000-7000-8000-000000000002";
let projects = [{ id: "p-notes-api", name: "notes-api", state: "ready", placed: true, created_at: "2026-10-19T00:00:00Z", environments: [{ id: "e0", name: "dev" }], workspaces: 1 }];
let found = [
  { name: "gold-homebase", path: "/home/dev/box-work/gold-homebase", git: true, remote: "https://github.com/example-org/gold-homebase.git", manifests: [] },
  { name: "guardrails", path: "/home/dev/box-work/guardrails", git: true, remote: "", manifests: ["go.mod"] },
  { name: "notes-site", path: "/home/dev/box-work/notes-site", git: false, manifests: ["package.json"] },
  { name: "notes-api", path: "/home/dev/box-work/projects/notes-api", git: true, remote: "", manifests: ["go.mod"], adopted: true, project_id: "p-notes-api" },
].map((f) => ({ adopted: false, ...f, slug: f.name }));
const routes = {
  "GET /v1/me": () => ({ user_id: "100000000000000001", display_name: "the operator H.", platform_roles: [], capabilities: {},
    tenants: [{ tenant_id: T, name: "the operator", roles: ["owner"], status: "active", can_manage: true }] }),
  [`GET /v1/tenants/${T}/projects`]: () => projects,
  [`GET /v1/tenants/${T}/workspaces`]: () => ({ can_manage: true, tenant_status: "active",
    items: [{ id: W, tenant_id: T, name: "dev", state: "running", ssh_port: 42001, previews: [] }] }),
  [`GET /v1/tenants/${T}/workspaces/${W}/discover`]: () => ({ items: found }),
  [`GET /v1/tenants/${T}/github/repos`]: () => ({ can_install: true, installations: [{ installation_id: 1, account_login: "example-org", suspended: false,
    repos: [{ id: 11, full_name: "example-org/gold-homebase", private: true, default_branch: "main" }, { id: 12, full_name: "example-org/rrflow", private: true, default_branch: "main" }] }] }),
};
http.createServer((req, res) => {
  const k = `${req.method} ${req.url.split("?")[0]}`;
  let body = "";
  req.on("data", (c) => (body += c));
  req.on("end", () => {
    if (k === `POST /v1/tenants/${T}/workspaces/${W}/adopt`) {
      const { path } = JSON.parse(body || "{}");
      const f = found.find((x) => x.path === path);
      f.adopted = true;
      projects.push({ id: "p-" + f.slug, name: f.slug, state: "ready", placed: true, created_at: new Date().toISOString(), environments: [{ id: "e1", name: "dev" }], workspaces: 1 });
      res.writeHead(201, { "content-type": "application/json" });
      return res.end(JSON.stringify({ slug: f.slug, rrflow: "queued", project: { id: "p-" + f.slug, public_id: "prj_01JAR0X3MQ8V5K2Z7T9B4N6C1D" } }));
    }
    if (k.endsWith("/github/publish")) {
      const f = found.find((x) => x.project_id === k.split("/")[5]);
      if (f) f.remote = "https://github.com/example-org/" + f.slug + ".git";
      res.writeHead(200, { "content-type": "application/json" });
      return res.end(JSON.stringify({ repo: "example-org/" + f.slug }));
    }
    const h = routes[k];
    if (!h) { console.log("miss", k); res.writeHead(404, { "content-type": "application/json" }); return res.end('{"message":"mock: not found"}'); }
    res.writeHead(200, { "content-type": "application/json" }); res.end(JSON.stringify(h()));
  });
}).listen(8080, "127.0.0.1", () => console.log("mock on 8080"));
