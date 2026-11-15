//go:build !linux

package hostd

import (
	"errors"
	"os"
	"os/exec"
)

// hostPTY is Linux only; elsewhere the framed terminal runs on pipes and ignores resize frames.
type hostPTY struct {
	master, slave *os.File
}

func openPTY(int, int) (*hostPTY, error) { return nil, errors.New("no host pseudo-terminal on this platform") }

func (p *hostPTY) resize(int, int) error { return errors.New("no host pseudo-terminal") }
func (p *hostPTY) attach(*exec.Cmd)      {}
func (p *hostPTY) close()                {}
