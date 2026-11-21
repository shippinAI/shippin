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
	"unicode/utf8"
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
	_, p, err := wsReadOp(r, w)
	return p, err
}

// wsReadOp is wsRead that also says whether the message was text (0x1) or binary (0x2). A
// continuation frame (0x0) is reported as binary: terminal input is binary and control messages
// are single small text frames.
func wsReadOp(r *bufio.Reader, w io.Writer) (byte, []byte, error) {
	for {
		var h [2]byte
		if _, err := io.ReadFull(r, h[:]); err != nil {
			return 0, nil, err
		}
		op, masked, n := h[0]&0x0f, h[1]&0x80 != 0, uint64(h[1]&0x7f)
		switch n {
		case 126:
			var b [2]byte
			if _, err := io.ReadFull(r, b[:]); err != nil {
				return 0, nil, err
			}
			n = uint64(binary.BigEndian.Uint16(b[:]))
		case 127:
			var b [8]byte
			if _, err := io.ReadFull(r, b[:]); err != nil {
				return 0, nil, err
			}
			n = binary.BigEndian.Uint64(b[:])
		}
		if !masked || n > wsMaxFrame {
			return 0, nil, errors.New("bad websocket frame")
		}
		var mask [4]byte
		if _, err := io.ReadFull(r, mask[:]); err != nil {
			return 0, nil, err
		}
		p := make([]byte, n)
		if _, err := io.ReadFull(r, p); err != nil {
			return 0, nil, err
		}
		for i := range p {
			p[i] ^= mask[i%4]
		}
		switch op {
		case 0x8:
			return 0, nil, io.EOF
		case 0x9:
			if err := wsWrite(w, 0xA, p); err != nil {
				return 0, nil, err
			}
		case 0x0:
			return 0x2, p, nil
		case 0x1, 0x2:
			return op, p, nil
		}
	}
}

// wsClose sends a close frame with a status code and a short reason, cut to the 123 bytes a
// close frame allows on a character boundary (a browser fails the socket on broken UTF-8).
func wsClose(w io.Writer, code int, reason string) error {
	for len(reason) > 123 {
		_, n := utf8.DecodeLastRuneInString(reason)
		reason = reason[:len(reason)-n]
	}
	return wsWrite(w, 0x8, append([]byte{byte(code >> 8), byte(code)}, reason...))
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
