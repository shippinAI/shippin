package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"shippin.cloud/console/internal/hostd"
)

// storageCmd talks to the running hostd over its unix socket.
//
//	shippin-hostd storage show --tenant UUID
//	shippin-hostd storage add --tenant UUID --quota-gib N --root-gib N --data-gib N
func storageCmd(cfg *hostd.Config, args []string) error {
	if len(args) < 1 || (args[0] != "show" && args[0] != "add") {
		return fmt.Errorf("usage: shippin-hostd storage show|add --tenant UUID [--quota-gib N --root-gib N --data-gib N]")
	}
	op := args[0]
	var tenant string
	var quota, root, data int
	for i := 1; i < len(args); i++ {
		need := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s needs a value", args[i])
			}
			i++
			return args[i], nil
		}
		switch args[i] {
		case "--tenant":
			v, err := need()
			if err != nil {
				return err
			}
			tenant = v
		case "--quota-gib", "--root-gib", "--data-gib":
			flag := args[i]
			v, err := need()
			if err != nil {
				return err
			}
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				return fmt.Errorf("%s: want a number of GiB", flag)
			}
			switch flag {
			case "--quota-gib":
				quota = n
			case "--root-gib":
				root = n
			case "--data-gib":
				data = n
			}
		default:
			return fmt.Errorf("unknown argument %s", args[i])
		}
	}
	if tenant == "" {
		return fmt.Errorf("--tenant is required")
	}
	var method, path = http.MethodGet, "/tenants/" + tenant + "/storage"
	var body io.Reader
	if op == "add" {
		if quota == 0 && root == 0 && data == 0 {
			return fmt.Errorf("storage add needs --quota-gib, --root-gib, or --data-gib")
		}
		method = http.MethodPost
		raw, _ := json.Marshal(map[string]int{"quota_gib": quota, "root_gib": root, "data_gib": data})
		body = bytes.NewReader(raw)
	}
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", cfg.Socket)
	}}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Minute}
	req, err := http.NewRequest(method, "http://hostd"+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		return fmt.Errorf("hostd: %s", bytes.TrimSpace(out))
	}
	var pretty bytes.Buffer
	if json.Indent(&pretty, out, "", "  ") != nil {
		fmt.Println(string(out))
		return nil
	}
	fmt.Println(pretty.String())
	return nil
}
