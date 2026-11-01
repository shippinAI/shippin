package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/gh"
	"shippin.cloud/console/internal/hostclient"
	"shippin.cloud/console/internal/projects"
)

// fakeGitHub answers token, repo-list and org-repo-create calls; fakeHost records hostd git ops.
func githubFlowAPI(t *testing.T) (*API, string, string, *[]hostclient.GitOp, int64) {
	a, d, tn := notifyAPI(t)
	ghSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/access_tokens"):
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "ghs_" + strings.Repeat("b", 36), "expires_at": time.Now().Add(time.Hour)})
		case r.URL.Path == "/installation/repositories":
			_, _ = w.Write([]byte(`{"repositories":[{"id":11,"full_name":"acme/Box_Work","default_branch":"main"},{"id":12,"full_name":"acme/empty","default_branch":""}]}`))
		case r.URL.Path == "/orgs/acme/repos":
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{"id":99,"full_name":"acme/notes-site","private":true,"default_branch":"main"}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(ghSrv.Close)
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	p := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	var err error
	if a.gh, err = gh.New("Iv1.t", "cs", string(p), ghSrv.URL, ghSrv.URL); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	ops := &[]hostclient.GitOp{}
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var op hostclient.GitOp
		_ = json.NewDecoder(r.Body).Decode(&op)
		mu.Lock()
		*ops = append(*ops, op)
		mu.Unlock()
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	t.Cleanup(host.Close)
	a.host = hostclient.New(host.URL)
	a.cfg.GitHub.MaxRepos = 100
	ctx := context.Background()
	ws := uuid.NewString()
	n, _ := rand.Int(rand.Reader, big.NewInt(20000))
	if _, err := d.Pool.Exec(ctx, `INSERT INTO workspaces(id,tenant_id,name,ssh_port,created_by,state) VALUES($1,$2,'dev',$3,'395290028818825218','running')`, ws, tn.ID, 40000+n.Int64()); err != nil {
		t.Fatal(err)
	}
	inst := 100000 + n.Int64()
	if err := d.BindInstallation(ctx, db.GitHubInstallation{ID: inst, TenantID: tn.ID, AccountLogin: "acme", AccountType: "Organization", RepoSelection: "all"}, ""); err != nil {
		t.Fatal(err)
	}
	return a, tn.ID, ws, ops, inst
}

func TestImportClonesThenOwns(t *testing.T) {
	a, tid, ws, ops, inst := githubFlowAPI(t)
	ctx := context.Background()
	repo := &gh.Repo{ID: 11, FullName: "acme/Box_Work", DefaultBranch: "main"}
	a.n2.tw = &fakeTW{err: errors.New("fga down")}
	if _, err := a.cloneAndOwn(ctx, tid, ws, "395290028818825218", inst, repo, "main"); err == nil {
		t.Fatal("tuple failure must fail the import")
	}
	if taken, _ := (&projects.Store{Pool: a.db.Pool}).SlugTaken(ctx, tid, "box-work"); taken {
		t.Fatal("import kept a project without ownership tuples")
	}
	tw := &fakeTW{}
	a.n2.tw = tw
	res, err := a.cloneAndOwn(ctx, tid, ws, "395290028818825218", inst, repo, "main")
	if err != nil || res.Slug != "box-work" || len(tw.got) != 7 {
		t.Fatalf("%+v %v tuples=%d", res, err, len(tw.got))
	}
	last := (*ops)[len(*ops)-1]
	if last.Op != "clone" || last.Path != "/home/dev/code/box-work" || last.Repo != "acme/Box_Work" || last.GitToken == nil {
		t.Fatalf("clone op %+v", last)
	}
	src, err := (&projects.Store{Pool: a.db.Pool}).SourceOf(ctx, tid, res.Project.ID)
	if err != nil || src.Remote != "https://github.com/acme/Box_Work.git" || src.WorkspaceID != ws {
		t.Fatalf("source %+v %v", src, err)
	}
}

func TestPublishCreatesRepoAndPushes(t *testing.T) {
	a, tid, ws, ops, inst := githubFlowAPI(t)
	ctx := context.Background()
	a.n2.tw = &fakeTW{}
	res, err := a.createOwned(ctx, ownedReq{TenantID: tid, WorkspaceID: ws, OwnerSub: "395290028818825218", By: "o",
		Found: hostclient.Found{Name: "notes-site", Path: "/home/dev/box-work/notes-site"}})
	if err != nil {
		t.Fatal(err)
	}
	st := &projects.Store{Pool: a.db.Pool}
	src, err := st.SourceOf(ctx, tid, res.Project.ID)
	if err != nil || src.Remote != "" {
		t.Fatalf("%+v %v", src, err)
	}
	repo := &gh.Repo{ID: 99, FullName: "acme/notes-site", DefaultBranch: "main"}
	if err := a.pushFolder(ctx, tid, src, inst, repo); err != nil {
		t.Fatal(err)
	}
	last := (*ops)[len(*ops)-1]
	if last.Op != "publish" || last.Path != "/home/dev/box-work/notes-site" || last.Repo != "acme/notes-site" || last.Branch != "main" {
		t.Fatalf("publish op %+v", last)
	}
	if src, _ = st.SourceOf(ctx, tid, res.Project.ID); src.Remote != "https://github.com/acme/notes-site.git" {
		t.Fatalf("remote not recorded: %+v", src)
	}
	if err := st.SetRemote(ctx, tid, res.Project.ID, "https://github.com/x/y.git"); err == nil {
		t.Fatal("a recorded remote must not be overwritten")
	}
	if r, _, msg := a.publishRepo(ctx, tid, inst, 0, "notes-site"); r == nil || r.ID != 99 {
		t.Fatalf("org create: %+v %s", r, msg)
	}
	if r, _, _ := a.publishRepo(ctx, tid, inst, 12, "x"); r == nil || r.FullName != "acme/empty" {
		t.Fatalf("link existing: %+v", r)
	}
	_ = a.db.BindInstallation(ctx, db.GitHubInstallation{ID: inst, TenantID: tid, AccountLogin: "operator", AccountType: "User"}, "")
	if _, code, msg := a.publishRepo(ctx, tid, inst, 0, "notes-site"); code != http.StatusUnprocessableEntity || !strings.Contains(msg, "notes-site") {
		t.Fatalf("personal account: %d %s", code, msg)
	}
	if _, code, _ := a.publishRepo(ctx, tid, 424242, 0, "notes-site"); code != http.StatusNotFound {
		t.Fatalf("foreign installation: %d", code)
	}
}
