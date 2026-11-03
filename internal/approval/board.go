package approval

import "context"

// Request is one approval posted to the factory board.
type Request struct {
	Key    string
	OrgID  string
	Action string
	Host   string
	Title  string
	Body   string
	By     string
}

// Decision is the board's current answer for a request key.
type Decision struct {
	Key    string `json:"key"`
	Status string `json:"status"`
	By     string `json:"by,omitempty"`
	Note   string `json:"note,omitempty"`
}

// Board posts approval requests and reads decisions. hub.Board talks to this console's
// agent checkout board. HTTPBoard talks to a remote board that implements the same routes.
type Board interface {
	Post(ctx context.Context, req Request) (Decision, error)
	Status(ctx context.Context, org, key string) (Decision, error)
}
