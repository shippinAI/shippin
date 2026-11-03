package ailaunch

import (
	"io"
	"sync"
)

// lockedWriter serializes pong frames with the PTY pump's frames.
type lockedWriter struct {
	w  io.Writer
	mu *sync.Mutex
}

func (l lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
