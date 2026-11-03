package backupcov

import "testing"

func TestSummarizeCountsRequiredGaps(t *testing.T) {
	items := []Item{
		{ID: "cfg", Required: true},
		{ID: "opt", Required: false},
	}
	s := Summarize(items, map[string]Observation{
		"cfg": {Present: true, Listed: true, Enabled: true, LastStatus: "failed"},
		"opt": {Present: true},
	})
	if s.RequiredGaps != 1 || s.RequiredReady != 0 || s.Present != 2 {
		t.Fatalf("%+v", s)
	}
}

func TestDestRejectsInlineSecrets(t *testing.T) {
	ok := Dest{Kind: "s3", Endpoint: "https://s3.example", Bucket: "b", SecretRef: "vaultix://org/proj/prod/s3"}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := Dest{Kind: "s3", Endpoint: "https://user:pass@s3.example", Bucket: "b", SecretRef: "vaultix://x"}
	if err := bad.Validate(); err == nil {
		t.Fatal("userinfo must be rejected")
	}
	if err := (Dest{Kind: "smb", Share: "//files/backups"}).Validate(); err == nil {
		t.Fatal("smb without a secret ref must fail")
	}
}

func TestRingCapsWithoutLogging(t *testing.T) {
	var r Ring
	r.Max = 2
	r.Append(Event{Message: "a"})
	r.Append(Event{Message: "b"})
	r.Append(Event{Message: "c"})
	if len(r.Items) != 2 || r.Items[0].Message != "c" {
		t.Fatalf("%+v", r.Items)
	}
}
