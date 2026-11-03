//go:build windows

package ailaunch

import (
	"errors"
	"os"
	"os/exec"
)

// The AI launcher needs a Unix pseudo-terminal, so it does not run on Windows. The console still
// builds there for development; only this feature reports that it is unavailable.
var errNoPTY = errors.New("ailaunch: a pseudo-terminal is not available on Windows")

func startPTY(*exec.Cmd, uint16, uint16) (*os.File, error) { return nil, errNoPTY }

func resize(*os.File, uint16, uint16) error { return errNoPTY }
