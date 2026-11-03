//go:build linux

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return r.Header.Get("Origin") == "http://"+r.Host ||
			r.Header.Get("Origin") == "https://"+r.Host ||
			r.Header.Get("Origin") == ""
	},
}

func handleTerminalPage(w http.ResponseWriter, r *http.Request) {
	hd := buildHeaderData(r)
	hd.PageTitle = "Terminal"
	data := struct {
		HeaderData
	}{
		HeaderData: hd,
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "terminal.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

type resizeMsg struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

func handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}

	username, _ := getSession(r)
	sudo := isSudoEnabled(r)

	var cmd *exec.Cmd
	if sudo {
		_, _, ok := getSessionPassword(r)
		if !ok {
			conn.WriteMessage(websocket.TextMessage, []byte("Session expired"))
			conn.Close()
			return
		}
		cmd = exec.Command("sudo", "-S", "-u", "root", "/bin/bash")
	} else {
		cmd = exec.Command("/bin/bash")
	}
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: 120, Rows: 40})
	if err != nil {
		log.Printf("pty start error: %v", err)
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to start shell: "+err.Error()))
		conn.Close()
		return
	}

	log.Printf("terminal opened for user %q (sudo=%v, pid=%d)", username, sudo, cmd.Process.Pid)

	// If sudo, authenticate non-interactively and suppress the password echo
	sudoReady := make(chan struct{})
	if sudo {
		_, password, _ := getSessionPassword(r)
		go func() {
			defer close(sudoReady)

			// Disable echo on PTY so the password isn't echoed back
			fd := ptmx.Fd()
			var orig syscall.Termios
			if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TCGETS, uintptr(unsafe.Pointer(&orig))); errno == 0 {
				noecho := orig
				noecho.Lflag &^= syscall.ECHO
				syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TCSETS, uintptr(unsafe.Pointer(&noecho)))
			}

			// Send the password to sudo -S via the PTY
			ptmx.Write([]byte(password + "\n"))

			// Wait for sudo to authenticate and start the shell
			time.Sleep(1 * time.Second)

			// Re-enable echo for normal terminal operation
			syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TCSETS, uintptr(unsafe.Pointer(&orig)))
		}()
	} else {
		close(sudoReady)
	}

	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			ptmx.Close()
			if cmd.Process != nil {
				syscall.Kill(-cmd.Process.Pid, syscall.SIGHUP)
				cmd.Wait()
			}
			log.Printf("terminal closed for user %q (pid=%d)", username, cmd.Process.Pid)
		})
	}
	defer cleanup()

	// Ping/pong for keepalive
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	var wsMu sync.Mutex

	// Goroutine: ping ticker
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			wsMu.Lock()
			err := conn.WriteMessage(websocket.PingMessage, nil)
			wsMu.Unlock()
			if err != nil {
				return
			}
		}
	}()

	// Goroutine: PTY -> WebSocket
	go func() {
		<-sudoReady // wait for sudo auth to complete before forwarding output
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if err != nil {
				wsMu.Lock()
				conn.WriteMessage(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				wsMu.Unlock()
				cleanup()
				return
			}
			wsMu.Lock()
			err = conn.WriteMessage(websocket.BinaryMessage, buf[:n])
			wsMu.Unlock()
			if err != nil {
				cleanup()
				return
			}
		}
	}()

	// Main loop: WebSocket -> PTY
	for {
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if len(msg) == 0 {
			continue
		}

		switch msgType {
		case websocket.BinaryMessage, websocket.TextMessage:
			// Check for control prefix
			if msg[0] == 1 && len(msg) > 1 {
				var rm resizeMsg
				if err := json.Unmarshal(msg[1:], &rm); err == nil && rm.Type == "resize" {
					pty.Setsize(ptmx, &pty.Winsize{
						Cols: rm.Cols,
						Rows: rm.Rows,
					})
				}
			} else {
				// Strip prefix byte if present
				data := msg
				if msg[0] == 0 && len(msg) > 1 {
					data = msg[1:]
				}
				ptmx.Write(data)
			}
		}
	}
}
