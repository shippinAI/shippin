package tenantdb

import (
	"context"
	"fmt"
	"strings"
)

// AskResult is the Ask AI hook. SQL stays nil until a model is connected.
type AskResult struct {
	Status  string  `json:"status"`
	Hook    string  `json:"hook"`
	Message string  `json:"message"`
	SQL     *string `json:"sql"`
}

// Asker drafts SQL for the tenant. The stub does not call a model.
type Asker interface {
	Ask(ctx context.Context, tenantID, actor, prompt string) (AskResult, error)
}

// StubAsker is the hook point for the tenant's AI subscription.
// Replace Ask with a call that uses that subscription; keep the route.
type StubAsker struct{}

func (StubAsker) Ask(_ context.Context, tenantID, _, prompt string) (AskResult, error) {
	if _, err := TargetOf(tenantID); err != nil {
		return AskResult{}, err
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return AskResult{}, fmt.Errorf("describe what you want the database to do")
	}
	if len(prompt) > 2000 {
		return AskResult{}, fmt.Errorf("that question is too long")
	}
	return AskResult{
		Status:  "stub",
		Hook:    "tenant_ai_subscription",
		Message: "Ask AI will use this organization's AI subscription to draft SQL. It isn't connected yet, so nothing was sent to a model.",
	}, nil
}
