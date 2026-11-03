package api

import (
	"context"
	"errors"
	"testing"
)

func TestOrgMemberFailsClosed(t *testing.T) {
	a := &API{}
	ctx := context.Background()
	a.orgOfUser = func(context.Context, string) (string, error) { return "395263804486713346", nil }
	if a.orgMember(ctx, "395263804486713346", "395290028818825218") != nil {
		t.Fatal("member refused")
	}
	if a.orgMember(ctx, "999", "395290028818825218") == nil {
		t.Fatal("other org accepted")
	}
	a.orgOfUser = func(context.Context, string) (string, error) { return "", errors.New("nomen down") }
	if a.orgMember(ctx, "395263804486713346", "395290028818825218") == nil {
		t.Fatal("lookup error must refuse")
	}
	for _, bad := range []string{"", "1/../../admin", "abc"} {
		if nomenSubRe.MatchString(bad) {
			t.Errorf("accepted sub %q", bad)
		}
	}
}
