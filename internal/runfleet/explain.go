package runfleet

import "strings"

// Explanation is a deterministic reading of a stored log. It does not call a model.
type Explanation struct {
	JobID      string   `json:"job_id"`
	Repo       string   `json:"repo"`
	Conclusion string   `json:"conclusion"`
	Summary    string   `json:"summary"`
	Signals    []string `json:"signals"`
	NextStep   string   `json:"next_step"`
	LogStored  bool     `json:"log_stored"`
	Redactions int      `json:"redactions"`
	Expired    bool     `json:"expired,omitempty"`
}

// ExplainLog classifies a job from its conclusion and log text.
func ExplainLog(job Job, log string, stored, expired bool) Explanation {
	out := Explanation{
		JobID: job.ID, Repo: job.Repo, Conclusion: job.Conclusion,
		LogStored: stored, Redactions: job.Redactions, Expired: expired,
		Signals: []string{},
	}
	low := strings.ToLower(log)
	switch {
	case expired:
		out.Signals = append(out.Signals, "expired")
		out.Summary = "The log was discarded because it is older than this repository's retention."
		out.NextStep = "Raise retention before the next run if you need the log, or rerun the job."
	case !stored || strings.TrimSpace(log) == "":
		out.Signals = append(out.Signals, "no_log")
		if job.Conclusion == "success" {
			out.Summary = "The job succeeded and no log is stored."
			out.NextStep = "Nothing to fix from logs."
		} else {
			out.Summary = "No log is stored for this job, so the conclusion is the only signal."
			out.NextStep = "Push the job log to the control plane, or rerun with log storage enabled."
		}
	case strings.Contains(low, "no runner matching") || strings.Contains(low, "waiting for a runner"):
		out.Signals = append(out.Signals, "runner_label")
		out.Summary = "GitHub did not find a runner for the runs-on label."
		out.NextStep = "Check the label against shippin-<size> and that this repository was approved."
	case strings.Contains(low, "out of memory") || strings.Contains(low, "killed") && strings.Contains(low, "signal"):
		out.Signals = append(out.Signals, "out_of_memory")
		out.Summary = "The job was killed in a way that usually means it ran out of memory."
		out.NextStep = "Move the job to a larger size, or reduce the build's memory use."
	case strings.Contains(low, "fail") && (strings.Contains(low, "test") || strings.Contains(log, "--- FAIL")):
		out.Signals = append(out.Signals, "test_failure")
		out.Summary = "A test failed. The first error line is the place to start."
		out.NextStep = "Open the first failing test named in the log."
	case strings.Contains(log, "##[error]") || strings.Contains(low, "process completed with exit code"):
		out.Signals = append(out.Signals, "process_error")
		out.Summary = "The job process exited with an error."
		out.NextStep = "Read the first ##[error] line in the stored log."
	case job.Conclusion == "success" || job.Conclusion == "":
		out.Signals = append(out.Signals, "succeeded")
		out.Summary = "The job succeeded."
		out.NextStep = "No failure to explain."
	default:
		out.Signals = append(out.Signals, "unknown_failure")
		out.Summary = "The job did not succeed, and the log did not match a known pattern."
		out.NextStep = "Read the stored log from the end."
	}
	if line := firstErrorLine(log); line != "" && out.Signals[0] != "no_log" && out.Signals[0] != "expired" {
		out.Summary = out.Summary + " First error: " + line
	}
	return out
}

func firstErrorLine(log string) string {
	for _, line := range strings.Split(log, "\n") {
		trim := strings.TrimSpace(line)
		if strings.Contains(trim, "##[error]") || strings.HasPrefix(trim, "FAIL") || strings.Contains(trim, "--- FAIL") {
			if len(trim) > 180 {
				trim = trim[:180] + "…"
			}
			return trim
		}
	}
	return ""
}
