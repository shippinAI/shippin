package runfleet

import "testing"

func TestExplainLogPatterns(t *testing.T) {
	job := Job{ID: "j1", Repo: "octo/hello", Conclusion: "failure"}
	cases := []struct {
		name   string
		log    string
		signal string
	}{
		{"label", "Waiting for a runner to pick up this job", "runner_label"},
		{"oom", "process killed with signal: killed", "out_of_memory"},
		{"test", "--- FAIL: TestBuild\nFAIL", "test_failure"},
		{"error", "##[error]Process completed with exit code 1", "process_error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExplainLog(job, c.log, true, false)
			if len(got.Signals) != 1 || got.Signals[0] != c.signal {
				t.Fatalf("signals = %#v", got.Signals)
			}
			if got.Summary == "" || got.NextStep == "" {
				t.Fatal("summary and next step are required")
			}
		})
	}
	empty := ExplainLog(job, "", false, false)
	if empty.Signals[0] != "no_log" {
		t.Fatal(empty.Signals)
	}
	old := ExplainLog(job, "secret", false, true)
	if old.Signals[0] != "expired" || old.Summary == "secret" {
		t.Fatal(old)
	}
}
