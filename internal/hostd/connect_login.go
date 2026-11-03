package hostd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

// LoginState is all the console ever learns about a browser sign-in in progress.
type LoginState struct {
	Provider  string    `json:"provider"`
	State     string    `json:"state"` // starting | waiting | connected | failed | expired
	URL       string    `json:"verification_url,omitempty"`
	Code      string    `json:"user_code,omitempty"`
	CodeInput bool      `json:"code_input"`
	StartedAt time.Time `json:"started_at"`
}

type loginSession struct {
	mu     sync.Mutex
	st     LoginState
	out    bytes.Buffer
	stdin  io.WriteCloser
	cancel context.CancelFunc
}

var logins sync.Map // "<w>/<provider>" -> *loginSession

// Write scans CLI output for the URL/code; output past connect.output_max_bytes is dropped.
func (l *loginSession) scan(p ConnectProvider, max int) io.Writer {
	return writerFunc(func(b []byte) (int, error) {
		l.mu.Lock()
		defer l.mu.Unlock()
		if room := max - l.out.Len(); room > 0 {
			l.out.Write(b[:min(len(b), room)])
		}
		if u, c := loginPrompt(p, l.out.String()); u != "" {
			l.st.URL, l.st.Code, l.st.State = u, c, "waiting"
		}
		return len(b), nil
	})
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(b []byte) (int, error) { return f(b) }

// StartLogin runs the provider's browser/device login as the user inside workspace w. One
// sign-in per provider per workspace; a new start replaces the previous one.
func (s *Server) StartLogin(ctx context.Context, t, w, id string) (*LoginState, error) {
	p, err := s.connectProvider(id)
	if err != nil {
		return nil, err
	}
	at, err := s.placeOf(ctx, t, w)
	if err != nil {
		return nil, err
	}
	key := w + "/" + id
	if old, ok := logins.LoadAndDelete(key); ok {
		old.(*loginSession).cancel()
	}
	c := s.cfg.Connect
	lctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.LoginSeconds)*time.Second)
	l := &loginSession{cancel: cancel, st: LoginState{Provider: id, State: "starting", CodeInput: p.CodeInput, StartedAt: time.Now().UTC()}}
	cmd := streamCmd(lctx, at, s.userExec(Instance(w), true, p.Login...))
	cmd.Stdout, cmd.Stderr = l.scan(p, c.OutputMax), l.scan(p, c.OutputMax)
	if l.stdin, err = cmd.StdinPipe(); err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("sign-in did not start")
	}
	if p.PressEnter {
		_, _ = io.WriteString(l.stdin, "\n")
	}
	logins.Store(key, l)
	go s.finishLogin(at, w, id, l, cmd.Wait, lctx)
	st := l.state()
	return &st, nil
}

func (s *Server) finishLogin(at place, w, id string, l *loginSession, wait func() error, ctx context.Context) {
	err := wait()
	l.mu.Lock()
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		l.st.State = "expired"
	case err != nil:
		l.st.State = "failed"
	default:
		l.st.State = "connected"
	}
	l.st.URL, l.st.Code = "", ""
	l.out.Reset() // never keep CLI output once the flow ends
	l.mu.Unlock()
	l.cancel()
	_ = s.kickWriter(context.Background(), at, Instance(w))
}

func (l *loginSession) state() LoginState {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.st
}

// Login reports the sign-in in progress for w/provider.
func (s *Server) Login(w, id string) (*LoginState, error) {
	v, ok := logins.Load(w + "/" + id)
	if !ok {
		return nil, fmt.Errorf("no sign-in in progress")
	}
	st := v.(*loginSession).state()
	return &st, nil
}

// SubmitCode types the code the vendor's page showed into the waiting CLI (stdin, never argv).
func (s *Server) SubmitCode(w, id, code string) error {
	v, ok := logins.Load(w + "/" + id)
	if !ok || !pasteCodeRe.MatchString(code) {
		return fmt.Errorf("no sign-in waiting for a code")
	}
	_, err := io.WriteString(v.(*loginSession).stdin, code+"\r")
	return err
}
