package mcpl

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"
)

// Event is what an action did, published once its change is committed. Its ID is the action's with
// the verb in the past: shippin.identity.ssh_key.create emits shippin.identity.ssh_key.created.
//
// An event carries who, where and what it acted on, never the action's output: a subscriber that
// needs the item reads it through a get action, under its own permissions.
type Event struct {
	ID     string `json:"id"`
	Action string `json:"action"`
	Actor  string `json:"actor,omitempty"`
	Tenant string `json:"tenant,omitempty"`
	// Access and Perm are the action's, so a stream shows the event only to people who could run it.
	Access  Access            `json:"access"`
	Perm    string            `json:"perm,omitempty"`
	Subject map[string]string `json:"subject,omitempty"`
	At      time.Time         `json:"at"`
}

// EventFor is the event an action emits: its ID with the verb as a past participle.
func EventFor(id string) string {
	i := strings.LastIndex(id, ".")
	return id[:i+1] + PastTense(id[i+1:])
}

var irregular = map[string]string{
	"begin": "begun", "bind": "bound", "build": "built", "buy": "bought", "choose": "chosen", "cut": "cut",
	"find": "found", "get": "got", "give": "given", "hold": "held", "keep": "kept", "lead": "led",
	"leave": "left", "make": "made", "pay": "paid", "put": "put", "quit": "quit", "read": "read",
	"rebuild": "rebuilt", "rerun": "rerun", "reset": "reset", "run": "run", "sell": "sold", "send": "sent",
	"resend": "resent", "set": "set", "shut": "shut", "split": "split", "take": "taken", "tell": "told",
	"undo": "undone", "withdraw": "withdrawn", "write": "written", "overwrite": "overwritten",
}

// doubled are verbs whose last consonant doubles: stop -> stopped.
var doubled = map[string]bool{"ban": true, "chat": true, "drop": true, "flag": true, "log": true, "map": true,
	"pin": true, "plan": true, "plug": true, "scan": true, "ship": true, "skip": true, "step": true,
	"stop": true, "swap": true, "tag": true, "trim": true, "wrap": true, "unpin": true, "unplug": true}

// PastTense is the past participle of a verb; for take_over only the first word changes: taken_over.
func PastTense(verb string) string {
	first, rest, multi := strings.Cut(verb, "_")
	p := first
	switch {
	case irregular[first] != "":
		p = irregular[first]
	case doubled[first]:
		p = first + first[len(first)-1:] + "ed"
	case strings.HasSuffix(first, "e"):
		p = first + "d"
	case strings.HasSuffix(first, "y") && len(first) > 1 && !strings.ContainsAny(first[len(first)-2:len(first)-1], "aeiou"):
		p = first[:len(first)-1] + "ied"
	default:
		p = first + "ed"
	}
	if multi {
		return p + "_" + rest
	}
	return p
}

// Bus delivers events inside the process: handlers react to one event (push SSH keys to running
// workspaces), streams follow all of them (an AI client waiting for something to happen).
type Bus struct {
	mu   sync.RWMutex
	on   map[string][]func(context.Context, Event)
	subs map[int]chan Event
	next int
}

// On runs fn for every event with this ID, in its own goroutine, after the change is committed.
func (b *Bus) On(id string, fn func(context.Context, Event)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.on == nil {
		b.on = map[string][]func(context.Context, Event){}
	}
	b.on[id] = append(b.on[id], fn)
}

// Subscribe follows every event until cancel is called. A subscriber that falls behind loses events
// rather than holding up the actions that publish them.
func (b *Bus) Subscribe() (<-chan Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subs == nil {
		b.subs = map[int]chan Event{}
	}
	id, ch := b.next, make(chan Event, 64)
	b.next++
	b.subs[id] = ch
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if c, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(c)
		}
	}
}

// Publish hands e to its handlers and every subscriber.
func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, fn := range b.on[e.ID] {
		go func() {
			defer func() {
				if p := recover(); p != nil {
					log.Printf("event %s: handler panicked: %v", e.ID, p)
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			fn(ctx, e)
		}()
	}
	for _, ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}
