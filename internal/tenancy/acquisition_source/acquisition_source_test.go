package acquisition_source

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"shippin.cloud/console/internal/acquisition"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/servers"
	"strings"
	"testing"
)

type commandProbe struct {
	acquisition.Repository
	calls int
	seed  []byte
	fail  error
}

func (p *commandProbe) Create(_ context.Context, _ mcpl.DB, in acquisition.CreateCommand, _ acquisition.Sealer) (acquisition.SourceSummary, error) {
	p.calls++
	p.seed = in.SigningSeed
	return acquisition.SourceSummary{ID: "fixture"}, p.fail
}
func (p *commandProbe) Update(_ context.Context, _ mcpl.DB, _ string, in acquisition.UpdateCommand, _ acquisition.Sealer) (acquisition.SourceSummary, error) {
	p.calls++
	p.seed = in.SigningSeed
	return acquisition.SourceSummary{ID: "fixture"}, p.fail
}
func actionCall(t *testing.T, p *commandProbe) *mcpl.Call {
	t.Helper()
	s, e := servers.NewSealer(strings.Repeat("11", 32))
	if e != nil {
		t.Fatal(e)
	}
	services := mcpl.Services{}
	mcpl.Provide[acquisition.Repository](services, p)
	mcpl.Provide[acquisition.Sealer](services, s)
	return &mcpl.Call{Ctx: context.Background(), Services: services}
}
func TestAcquisitionCommandBoundaries(t *testing.T) {
	raw := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 32)))
	for _, bad := range []string{"", raw + "\n", raw + " ", "not-a-key", base64.StdEncoding.EncodeToString([]byte("short"))} {
		p := &commandProbe{}
		_, e := Create.Run(actionCall(t, p), CreateInput{SigningKey: bad})
		if e == nil || p.calls != 0 {
			t.Fatal("invalid seed reached repository")
		}
	}
	p := &commandProbe{}
	out, e := Create.Run(actionCall(t, p), CreateInput{SigningKey: raw})
	if e != nil || p.calls != 1 || len(p.seed) != 32 {
		t.Fatal("valid seed not processed")
	}
	for _, b := range p.seed {
		if b != 0 {
			t.Fatal("decoded signing seed retained after action")
		}
	}
	body, _ := json.Marshal(out)
	if strings.Contains(string(body), raw) || strings.Contains(string(body), "signing_key") {
		t.Fatal("write-only signer returned")
	}
	paused := false
	for _, in := range []UpdateInput{{ExpectedConfigGeneration: 1}, {Paused: &paused}, {ExpectedConfigGeneration: -1, Paused: &paused}} {
		p := &commandProbe{}
		_, e := Update.Run(actionCall(t, p), in)
		if e == nil || p.calls != 0 {
			t.Fatal("missing generation/pause reached repository")
		}
	}
	p = &commandProbe{}
	_, e = Update.Run(actionCall(t, p), UpdateInput{ExpectedConfigGeneration: 1, Paused: &paused})
	if e != nil || p.calls != 1 || p.seed != nil {
		t.Fatal("blank update signer must preserve existing credential")
	}
	p = &commandProbe{fail: errors.New("sensitive backend fixture detail")}
	_, e = Create.Run(actionCall(t, p), CreateInput{SigningKey: raw})
	if e == nil || strings.Contains(e.Error(), "sensitive backend") {
		t.Fatal("repository failure not sanitized")
	}
}
