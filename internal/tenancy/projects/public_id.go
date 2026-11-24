package projects

import (
	"crypto/rand"
	"encoding/binary"
	"regexp"
	"time"
)

// crockford is the ULID alphabet (no I, L, O, U).
const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

var publicIDShape = regexp.MustCompile(`^prj_[0-9A-HJKMNP-TV-Z]{26}$`)

// NewPublicID returns prj_<ULID>: 48-bit ms timestamp + 80 random bits, Crockford base32,
// lexically sortable by creation time. It never changes when the slug is renamed.
func NewPublicID() string { return "prj_" + ulidAt(time.Now()) }

// ValidPublicID reports whether s has the prj_<ULID> shape.
func ValidPublicID(s string) bool { return publicIDShape.MatchString(s) }

func ulidAt(t time.Time) string {
	var b [16]byte
	ms := uint64(t.UnixMilli())
	var ts [8]byte
	binary.BigEndian.PutUint64(ts[:], ms)
	copy(b[:6], ts[2:])
	_, _ = rand.Read(b[6:])
	// 128 bits -> 26 base32 chars, most significant first (first char carries 3 bits).
	out := make([]byte, 26)
	hi := binary.BigEndian.Uint64(b[:8])
	lo := binary.BigEndian.Uint64(b[8:])
	for i := 25; i >= 0; i-- {
		out[i] = crockford[lo&31]
		lo = lo>>5 | hi<<59
		hi >>= 5
	}
	return string(out)
}
