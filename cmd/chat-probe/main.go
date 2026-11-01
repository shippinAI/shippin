// Command chat-probe runs one real chat turn through the same code path as the console (chat
// store, hostd chat command, the provider's CLI in the workspace) without any browser session:
// the session is owned by a probe identity, never by a person. Run it in the console container.
//
//	chat-probe -tenant <uuid> -workspace <uuid> -provider claude -text "hello"
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"shippin.cloud/console/internal/chat"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostclient"
)

func main() {
	tenant := flag.String("tenant", "", "tenant uuid")
	ws := flag.String("workspace", "", "workspace uuid")
	prov := flag.String("provider", "", "chat provider id")
	text := flag.String("text", "", "message")
	owner := flag.String("owner", "probe:chat", "session owner (never a person's IdP subject)")
	sess := flag.String("session", "", "continue this probe-owned session instead of creating one")
	attach := flag.Bool("attach-rrflow", false, "ensure + bind the project's rrflow/state to the workspace first")
	check := flag.Bool("check", false, "run the provider's readiness check instead of a turn")
	flag.Parse()
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	var p config.ChatProvider
	for _, x := range cfg.Chat.Providers {
		if x.ID == *prov {
			p = x
		}
	}
	if p.ID == "" || *tenant == "" || *ws == "" {
		log.Fatal("-tenant, -workspace and a configured -provider are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Chat.TimeoutSeconds)*time.Second)
	defer cancel()
	host := hostclient.New(cfg.Incus.HostdURL)
	if *check {
		p.Command = p.Check
		r := chat.Turn(ctx, host, cfg.Chat, p, *tenant, *ws, "", "", func(string) {})
		fmt.Printf("ready=%v detail=%q\n", r.Err == "", r.Err)
		return
	}
	d, err := db.Open(ctx, os.Getenv("SHIPPIN_DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	if *attach {
		if err := attachRRFlow(ctx, d, host, *tenant, *ws, *owner); err != nil {
			log.Fatal("rrflow attach: ", err)
		}
	}
	st := &chat.Store{Pool: d.Pool}
	var s *chat.Session
	if *sess != "" {
		s, err = st.Get(ctx, *owner, *sess)
	} else {
		s, err = st.Create(ctx, chat.Session{TenantID: *tenant, WorkspaceID: *ws, Owner: *owner, Provider: p.ID})
	}
	if err != nil || s == nil {
		log.Fatal(err)
	}
	fmt.Printf("session %s (owner %s)\n", s.ID, *owner)
	if _, err := st.Add(ctx, s.ID, "user", *text, nil); err != nil {
		log.Fatal(err)
	}
	r := chat.Turn(ctx, host, cfg.Chat, p, *tenant, *ws, s.ID, chat.Prompt(cfg.Chat, nil, *text), func(d string) { fmt.Printf("delta: %q\n", d) })
	role, body := "assistant", r.Text
	if r.Err != "" {
		role, body, r.Blocks = "error", r.Err, chat.ConnectBlocks(p.Name)
	}
	m, err := st.Add(ctx, s.ID, role, body, r.Blocks)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("stored %s message %s: %q, a2ui blocks: %d\n", role, m.ID, body, len(r.Blocks))
}
