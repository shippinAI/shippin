// Command rrflow-state is the rrflow/state managed service (02c-rrflow.md): it runs in a service
// instance si-<uuid> on the tenant's boundary VM. Invoked as "rrflow" it is the client that bound
// workspaces use (RRFLOW_URL + the binding token file).
//
//	rrflow-state serve                         (env RRFLOW_LISTEN, RRFLOW_DATA, RRFLOW_ORG)
//	rrflow-state bind <binding> <workspace> [namespace]   (token on stdin; only its hash is kept)
//	rrflow-state unbind <binding>
//	rrflow-state export <sessions|memory|decisions> [limit] [namespace]
//
// One instance per tenant VM (Notion D36) keeps each project's state in its own namespace.
//
//	rrflow-state verify
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"shippin.cloud/console/internal/rrflow/state"
	"shippin.cloud/console/internal/rrflow/stateapi"
)

func main() {
	if filepath.Base(os.Args[0]) == "rrflow" {
		os.Exit(client(os.Args[1:]))
	}
	if len(os.Args) < 2 {
		die("usage: rrflow-state serve|bind|unbind|export|verify")
	}
	data := env("RRFLOW_DATA", "/var/lib/rrflow")
	b := &stateapi.Bindings{Path: filepath.Join(data, "bindings.json")}
	switch os.Args[1] {
	case "serve":
		ns := &state.Namespaces{Root: data, Org: os.Getenv("RRFLOW_ORG")}
		if _, err := ns.Get(""); err != nil {
			die(err.Error())
		}
		srv := &http.Server{Addr: env("RRFLOW_LISTEN", ":7710"), Handler: (&stateapi.Server{Stores: ns, Bindings: b}).Routes(),
			ReadHeaderTimeout: 10 * time.Second}
		log.Printf("rrflow-state: org %s listening on %s", ns.Org, srv.Addr)
		log.Fatal(srv.ListenAndServe())
	case "bind":
		if len(os.Args) != 4 && len(os.Args) != 5 {
			die("usage: rrflow-state bind <binding> <workspace> [namespace] < token")
		}
		tok, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		check(os.MkdirAll(data, 0o700))
		check(b.Set(os.Args[2], os.Args[3], arg(4), strings.TrimSpace(tok)))
	case "unbind":
		if len(os.Args) != 3 {
			die("usage: rrflow-state unbind <binding>")
		}
		check(b.Remove(os.Args[2]))
	case "export":
		export(open(data, arg(4)), os.Args[2:])
	case "verify":
		n, err := open(data, arg(2)).Verify()
		check(err)
		fmt.Printf("{\"decisions\":%d,\"chain\":\"ok\"}\n", n)
	default:
		die("unknown command " + os.Args[1])
	}
}

// export prints one kind as JSON for shippin-hostd (all workspaces of this instance).
func export(st *state.Store, args []string) {
	if len(args) < 1 {
		die("usage: rrflow-state export <sessions|memory|decisions> [limit]")
	}
	n := 100
	if len(args) > 1 {
		if v, err := strconv.Atoi(args[1]); err == nil && v > 0 && v <= 500 {
			n = v
		}
	}
	var out any
	switch args[0] {
	case "sessions":
		out = st.Sessions("", n)
	case "memory":
		out = st.Memory("", n)
	case "decisions":
		_, err := st.Verify()
		out = map[string]any{"items": st.Decisions("", n), "chain_ok": err == nil}
	default:
		die("unknown kind")
	}
	check(json.NewEncoder(os.Stdout).Encode(out))
}

// open is namespace ns's store ("" = the instance root).
func open(dir, ns string) *state.Store {
	st, err := (&state.Namespaces{Root: dir, Org: os.Getenv("RRFLOW_ORG")}).Get(ns)
	check(err)
	return st
}

func arg(i int) string {
	if len(os.Args) > i {
		return os.Args[i]
	}
	return ""
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func check(err error) {
	if err != nil {
		die(err.Error())
	}
}

func die(msg string) {
	fmt.Fprintln(os.Stderr, "rrflow-state:", msg)
	os.Exit(2)
}
