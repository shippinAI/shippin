package hostd

import (
	"encoding/binary"
	"errors"
	"io"
)

// TerminalFramedProto is the framed terminal stream. The console sends frames: one kind byte,
// a big-endian uint16 payload length, then the payload. hostd sends raw output bytes back.
// TermData carries keystrokes; TermResize carries rows then cols as two big-endian uint16.
// An unknown kind or a size out of bounds is a protocol error and ends the session.
const TerminalFramedProto = "shippin-pty.2"

// TerminalResizeHeader is set on hostd's 101 for the framed stream: "pty" when a resize frame
// reaches the pseudo-terminal, "none" when hostd could not open one and ignores resize frames.
const TerminalResizeHeader = "Shippin-Resize"

const (
	TermData   byte = 0
	TermResize byte = 1

	TermMaxRows = 500
	TermMaxCols = 1000

	termMaxChunk = 0xffff
)

// TermFrame is one decoded console frame.
type TermFrame struct {
	Kind       byte
	Data       []byte
	Rows, Cols int
}

var errTermFrame = errors.New("bad terminal frame")

// ValidTermSize reports whether rows x cols is a window size hostd accepts.
func ValidTermSize(rows, cols int) bool {
	return rows >= 1 && rows <= TermMaxRows && cols >= 1 && cols <= TermMaxCols
}

// WriteTermData frames p as TermData, split into chunks the length field can carry.
func WriteTermData(w io.Writer, p []byte) error {
	for len(p) > 0 {
		n := min(len(p), termMaxChunk)
		if err := writeTermFrame(w, TermData, p[:n]); err != nil {
			return err
		}
		p = p[n:]
	}
	return nil
}

// WriteTermResize frames a window size; out-of-bounds sizes are refused before they are sent.
func WriteTermResize(w io.Writer, rows, cols int) error {
	if !ValidTermSize(rows, cols) {
		return errTermFrame
	}
	var b [4]byte
	binary.BigEndian.PutUint16(b[:2], uint16(rows))
	binary.BigEndian.PutUint16(b[2:], uint16(cols))
	return writeTermFrame(w, TermResize, b[:])
}

func writeTermFrame(w io.Writer, kind byte, p []byte) error {
	h := []byte{kind, byte(len(p) >> 8), byte(len(p))}
	_, err := w.Write(append(h, p...))
	return err
}

// ReadTermFrame reads one frame. io.EOF is returned only on a clean end between frames.
func ReadTermFrame(r io.Reader) (TermFrame, error) {
	var h [3]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return TermFrame{}, errTermFrame
		}
		return TermFrame{}, err
	}
	p := make([]byte, binary.BigEndian.Uint16(h[1:]))
	if _, err := io.ReadFull(r, p); err != nil {
		return TermFrame{}, errTermFrame
	}
	switch h[0] {
	case TermData:
		return TermFrame{Kind: TermData, Data: p}, nil
	case TermResize:
		if len(p) != 4 {
			return TermFrame{}, errTermFrame
		}
		f := TermFrame{Kind: TermResize, Rows: int(binary.BigEndian.Uint16(p[:2])), Cols: int(binary.BigEndian.Uint16(p[2:]))}
		if !ValidTermSize(f.Rows, f.Cols) {
			return TermFrame{}, errTermFrame
		}
		return f, nil
	}
	return TermFrame{}, errTermFrame
}
