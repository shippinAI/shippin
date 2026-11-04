package approval

// Decision is the state returned by the local agent task board. It is not an
// authorization decision for provider changes; those require AgentHub review.
type Decision struct {
	Key    string `json:"key"`
	Status string `json:"status"`
	By     string `json:"by,omitempty"`
	Note   string `json:"note,omitempty"`
}
