package runfleet

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrNotConfigured means the GitHub App credentials are absent.
var ErrNotConfigured = errors.New("github app is not configured")

// RepoFact is what onboarding needs to know about a repository.
type RepoFact struct {
	FullName      string         `json:"full_name"`
	Private       bool           `json:"private"`
	License       string         `json:"license,omitempty"`
	Lovable       bool           `json:"lovable"`
	DefaultBranch string         `json:"default_branch,omitempty"`
	Workflows     []WorkflowFile `json:"workflows,omitempty"`
}

// GitHub is the only GitHub boundary. Tests use a fake; serve uses HTTPGitHub.
type GitHub interface {
	Configured() bool
	Repo(ctx context.Context, full string) (RepoFact, error)
	OpenPull(ctx context.Context, repo, title, body, branch string, files []FileChange) (string, error)
	RunnerStates(ctx context.Context, repos []string) (map[string]string, error)
}

// HTTPGitHub talks to the GitHub REST API. Token is an installation token.
// When Token is empty and a private key is set, calls mint one for Account.
type HTTPGitHub struct {
	BaseURL        string
	Token          string
	AppID          string
	PrivateKey     *rsa.PrivateKey
	InstallationID int64
	Client         *http.Client
}

func (h *HTTPGitHub) Configured() bool {
	return h != nil && (h.Token != "" || (h.AppID != "" && h.PrivateKey != nil && h.InstallationID != 0))
}

func (h *HTTPGitHub) Repo(ctx context.Context, full string) (RepoFact, error) {
	if !h.Configured() {
		return RepoFact{}, ErrNotConfigured
	}
	var body struct {
		FullName      string `json:"full_name"`
		Private       bool   `json:"private"`
		DefaultBranch string `json:"default_branch"`
		License       *struct {
			SPDX string `json:"spdx_id"`
		} `json:"license"`
	}
	if err := h.get(ctx, "/repos/"+full, &body); err != nil {
		return RepoFact{}, err
	}
	fact := RepoFact{FullName: body.FullName, Private: body.Private, DefaultBranch: body.DefaultBranch}
	if body.License != nil && body.License.SPDX != "" && body.License.SPDX != "NOASSERTION" {
		fact.License = body.License.SPDX
	}
	var entries []struct {
		Name string `json:"name"`
		Type string `json:"type"`
		Path string `json:"path"`
	}
	if err := h.get(ctx, "/repos/"+full+"/contents/.github/workflows?ref="+body.DefaultBranch, &entries); err != nil && !isNotFound(err) {
		return RepoFact{}, err
	}
	for _, ent := range entries {
		if ent.Type != "file" || !strings.HasSuffix(ent.Name, ".yml") && !strings.HasSuffix(ent.Name, ".yaml") {
			continue
		}
		text, err := h.fileText(ctx, full, ent.Path, body.DefaultBranch)
		if err != nil {
			return RepoFact{}, err
		}
		fact.Workflows = append(fact.Workflows, WorkflowFile{Path: ent.Path, Content: text})
	}
	if _, err := h.fileText(ctx, full, ".lovable/project.json", body.DefaultBranch); err == nil {
		fact.Lovable = true
	} else if !isNotFound(err) {
		return RepoFact{}, err
	}
	return fact, nil
}

func (h *HTTPGitHub) OpenPull(ctx context.Context, repo, title, body, branch string, files []FileChange) (string, error) {
	if !h.Configured() {
		return "", ErrNotConfigured
	}
	var meta struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := h.get(ctx, "/repos/"+repo, &meta); err != nil {
		return "", err
	}
	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	if err := h.get(ctx, "/repos/"+repo+"/git/ref/heads/"+meta.DefaultBranch, &ref); err != nil {
		return "", err
	}
	var treeItems []map[string]string
	for _, file := range files {
		var blob struct {
			SHA string `json:"sha"`
		}
		if err := h.post(ctx, "/repos/"+repo+"/git/blobs", map[string]string{"content": file.Content, "encoding": "utf-8"}, &blob); err != nil {
			return "", err
		}
		treeItems = append(treeItems, map[string]string{"path": file.Path, "mode": "100644", "type": "blob", "sha": blob.SHA})
	}
	var tree struct {
		SHA string `json:"sha"`
	}
	if err := h.post(ctx, "/repos/"+repo+"/git/trees", map[string]any{"base_tree": ref.Object.SHA, "tree": treeItems}, &tree); err != nil {
		return "", err
	}
	var commit struct {
		SHA string `json:"sha"`
	}
	if err := h.post(ctx, "/repos/"+repo+"/git/commits", map[string]any{
		"message": title, "tree": tree.SHA, "parents": []string{ref.Object.SHA},
	}, &commit); err != nil {
		return "", err
	}
	if err := h.post(ctx, "/repos/"+repo+"/git/refs", map[string]string{
		"ref": "refs/heads/" + branch, "sha": commit.SHA,
	}, &struct{}{}); err != nil {
		return "", err
	}
	var pull struct {
		HTMLURL string `json:"html_url"`
	}
	if err := h.post(ctx, "/repos/"+repo+"/pulls", map[string]string{
		"title": title, "body": body, "head": branch, "base": meta.DefaultBranch,
	}, &pull); err != nil {
		return "", err
	}
	if pull.HTMLURL == "" {
		return "", fmt.Errorf("github did not return a pull request url")
	}
	return pull.HTMLURL, nil
}

func (h *HTTPGitHub) RunnerStates(ctx context.Context, repos []string) (map[string]string, error) {
	if !h.Configured() {
		return nil, ErrNotConfigured
	}
	out := map[string]string{}
	for _, repo := range repos {
		var body struct {
			Runners []struct {
				Name   string `json:"name"`
				Status string `json:"status"`
				Busy   bool   `json:"busy"`
			} `json:"runners"`
		}
		if err := h.get(ctx, "/repos/"+repo+"/actions/runners?per_page=100", &body); err != nil {
			return nil, err
		}
		for _, ru := range body.Runners {
			state := ru.Status
			if ru.Busy {
				state += "-busy"
			}
			out[repo+"|"+ru.Name] = state
		}
	}
	return out, nil
}

func (h *HTTPGitHub) fileText(ctx context.Context, repo, path, ref string) (string, error) {
	var body struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	q := "/repos/" + repo + "/contents/" + path
	if ref != "" {
		q += "?ref=" + ref
	}
	if err := h.get(ctx, q, &body); err != nil {
		return "", err
	}
	if body.Encoding != "base64" {
		return "", fmt.Errorf("unexpected encoding %q", body.Encoding)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(body.Content, "\n", ""))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

type apiError struct {
	Status int
	Body   string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("github HTTP %d: %s", e.Status, e.Body)
}

func isNotFound(err error) bool {
	var api *apiError
	return errors.As(err, &api) && api.Status == http.StatusNotFound
}

func (h *HTTPGitHub) get(ctx context.Context, path string, dest any) error {
	return h.do(ctx, http.MethodGet, path, nil, dest)
}

func (h *HTTPGitHub) post(ctx context.Context, path string, payload, dest any) error {
	return h.do(ctx, http.MethodPost, path, payload, dest)
}

func (h *HTTPGitHub) do(ctx context.Context, method, path string, payload, dest any) error {
	token, err := h.token(ctx)
	if err != nil {
		return err
	}
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = strings.NewReader(string(b))
	}
	base := h.BaseURL
	if base == "" {
		base = "https://api.github.com"
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(base, "/")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		msg, _ := Redact(string(data))
		if len(msg) > 180 {
			msg = msg[:180]
		}
		return &apiError{Status: resp.StatusCode, Body: msg}
	}
	if dest == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, dest)
}

func (h *HTTPGitHub) token(ctx context.Context) (string, error) {
	if h.Token != "" {
		return h.Token, nil
	}
	if h.AppID == "" || h.PrivateKey == nil || h.InstallationID == 0 {
		return "", ErrNotConfigured
	}
	jwt, err := AppJWT(h.AppID, h.PrivateKey, time.Now())
	if err != nil {
		return "", err
	}
	base := h.BaseURL
	if base == "" {
		base = "https://api.github.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/app/installations/%d/access_tokens", strings.TrimRight(base, "/"), h.InstallationID), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")
	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var body struct {
		Token string `json:"token"`
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github installation token HTTP %d", resp.StatusCode)
	}
	if err := json.Unmarshal(data, &body); err != nil || body.Token == "" {
		return "", fmt.Errorf("github installation token response was empty")
	}
	return body.Token, nil
}

// AppJWT signs a GitHub App assertion. The key never leaves this process.
func AppJWT(appID string, key *rsa.PrivateKey, now time.Time) (string, error) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload, _ := json.Marshal(map[string]any{
		"iat": now.Add(-60 * time.Second).Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": appID,
	})
	body := header + "." + base64.RawURLEncoding.EncodeToString(payload)
	sum := sha256.Sum256([]byte(body))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return body + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// VerifyWebhook checks X-Hub-Signature-256. A missing secret never validates.
func VerifyWebhook(secret string, signature string, body []byte) bool {
	if secret == "" || !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	mac := hmacSHA256([]byte(secret), body)
	return hmacEqual(signature, "sha256="+mac)
}
