//go:build linux

package hostd

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

// hostPTY is a pseudo-terminal on the host that the terminal's incus client runs in. incus exec
// reads its window size from its own terminal at start and, on SIGWINCH, sends the new size to
// the instance's PTY over its control channel. Setting the size on this master is what raises
// that SIGWINCH, so a resize frame from the console reaches the shell inside the workspace.
type hostPTY struct {
	master, slave *os.File
}

func openPTY(rows, cols int) (*hostPTY, error) {
	fd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	n, err := unix.IoctlGetInt(fd, unix.TIOCGPTN)
	if err == nil {
		err = unix.IoctlSetPointerInt(fd, unix.TIOCSPTLCK, 0)
	}
	if err != nil {
		unix.Close(fd)
		return nil, err
	}
	name := "/dev/pts/" + strconv.Itoa(n)
	sfd, err := unix.Open(name, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		unix.Close(fd)
		return nil, err
	}
	// O_NONBLOCK keeps the master on Go's poller, so closing it ends a blocked Read.
	p := &hostPTY{master: os.NewFile(uintptr(fd), "/dev/ptmx"), slave: os.NewFile(uintptr(sfd), name)}
	if err := p.resize(rows, cols); err != nil {
		p.close()
		return nil, err
	}
	return p, nil
}

// resize sets the window size on the master; the kernel signals the terminal's foreground group.
func (p *hostPTY) resize(rows, cols int) error {
	rc, err := p.master.SyscallConn()
	if err != nil {
		return err
	}
	var ierr error
	err = rc.Control(func(fd uintptr) {
		ierr = unix.IoctlSetWinsize(int(fd), unix.TIOCSWINSZ, &unix.Winsize{Row: uint16(rows), Col: uint16(cols)})
	})
	if err != nil {
		return err
	}
	return ierr
}

// attach makes the slave cmd's stdio and controlling terminal, in a session of its own.
func (p *hostPTY) attach(cmd *exec.Cmd) {
	cmd.Stdin, cmd.Stdout, cmd.Stderr = p.slave, p.slave, p.slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
}

func (p *hostPTY) close() {
	p.master.Close()
	p.slave.Close()
}
