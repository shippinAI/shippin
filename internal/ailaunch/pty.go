package ailaunch

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// startPTY runs cmd on a new pseudo-terminal of rows x cols and returns the master side.
func startPTY(cmd *exec.Cmd, rows, cols uint16) (*os.File, error) {
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	n, err := unix.IoctlGetInt(int(m.Fd()), unix.TIOCGPTN)
	if err == nil {
		var unlock int32
		_, _, e := syscall.Syscall(syscall.SYS_IOCTL, m.Fd(), unix.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock)))
		if e != 0 {
			err = e
		}
	}
	if err != nil {
		m.Close()
		return nil, err
	}
	s, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		m.Close()
		return nil, err
	}
	defer s.Close()
	_ = resize(m, rows, cols)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = s, s, s
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		m.Close()
		return nil, err
	}
	return m, nil
}

func resize(m *os.File, rows, cols uint16) error {
	return unix.IoctlSetWinsize(int(m.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: rows, Col: cols})
}
