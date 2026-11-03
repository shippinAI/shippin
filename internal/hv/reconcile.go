package hv

import "time"

// Step is one reconciler observation. Log is true only when Err changed, so a stuck
// host does not emit the same line on every pass.
type Step struct {
	Health Health
	Guests int
	Err    string
	Wait   time.Duration
	Log    bool
}

// NextWait doubles prev while the error string is unchanged, and resets to base when it changes.
func NextWait(prev time.Duration, err string, sameErr bool, base, max time.Duration) time.Duration {
	if err == "" || !sameErr {
		return base
	}
	if prev < base {
		prev = base
	}
	next := prev * 2
	if next > max || next < prev {
		return max
	}
	return next
}

// Observe records health and guest count. sameErr is true when err equals the previous error.
func Observe(h Health, guests int, err string, prevErr string, prevWait, base, max time.Duration) Step {
	same := err != "" && err == prevErr
	return Step{
		Health: h,
		Guests: guests,
		Err:    err,
		Wait:   NextWait(prevWait, err, same, base, max),
		Log:    err != prevErr,
	}
}
