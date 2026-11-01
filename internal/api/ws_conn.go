package api

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
)

// wsGUID is RFC 6455's fixed handshake suffix.
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// wsAccept completes a server-side WebSocket handshake (same-origin only) and hijacks the conn.
func wsAccept(w http.ResponseWriter, r *http.Request, origin string) (net.Conn, *bufio.ReadWriter, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") || r.Header.Get("Sec-WebSocket-Version") != "13" {
		return nil, nil, errors.New("not a websocket request")
	}
	if r.Header.Get("Origin") != origin {
		return nil, nil, errors.New("cross-origin websocket refused")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, nil, errors.New("missing websocket key")
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("websocket unsupported")
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return nil, nil, err
	}
	sum := sha1.Sum([]byte(key + wsGUID))
	_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " +
		base64.StdEncoding.EncodeToString(sum[:]) + "\r\n\r\n")
	return conn, rw, rw.Flush()
}

// wsMaxFrame bounds one client frame (keystrokes and pastes are small).
const wsMaxFrame = 1 << 20

// wsRead returns the next data payload from a client; pings are answered, close ends the stream.
func wsRead(r *bufio.Reader, w io.Writer) ([]byte, error) {
	for {
		var h [2]byte
		if _, err := io.ReadFull(r, h[:]); err != nil {
			return nil, err
		}
		op, masked, n := h[0]&0x0f, h[1]&0x80 != 0, uint64(h[1]&0x7f)
		switch n {
		case 126:
			var b [2]byte
			if _, err := io.ReadFull(r, b[:]); err != nil {
				return nil, err
			}
			n = uint64(binary.BigEndian.Uint16(b[:]))
		case 127:
			var b [8]byte
			if _, err := io.ReadFull(r, b[:]); err != nil {
				return nil, err
			}
			n = binary.BigEndian.Uint64(b[:])
		}
		if !masked || n > wsMaxFrame {
			return nil, errors.New("bad websocket frame")
		}
		var mask [4]byte
		if _, err := io.ReadFull(r, mask[:]); err != nil {
			return nil, err
		}
		p := make([]byte, n)
		if _, err := io.ReadFull(r, p); err != nil {
			return nil, err
		}
		for i := range p {
			p[i] ^= mask[i%4]
		}
		switch op {
		case 0x8:
			return nil, io.EOF
		case 0x9:
			if err := wsWrite(w, 0xA, p); err != nil {
				return nil, err
			}
		case 0x0, 0x1, 0x2:
			return p, nil
		}
	}
}

// wsWrite sends one unmasked server frame.
func wsWrite(w io.Writer, op byte, p []byte) error {
	h := []byte{0x80 | op}
	switch n := len(p); {
	case n < 126:
		h = append(h, byte(n))
	case n <= 0xffff:
		h = append(h, 126, byte(n>>8), byte(n))
	default:
		h = append(h, 127)
		h = binary.BigEndian.AppendUint64(h, uint64(n))
	}
	_, err := w.Write(append(h, p...))
	return err
}
