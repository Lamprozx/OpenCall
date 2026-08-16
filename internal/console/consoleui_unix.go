//go:build unix

package console

import "golang.org/x/sys/unix"

func ttyIsTerminal(fd int) bool {
	_, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	return err == nil
}

func ttyMakeRaw(fd int) (any, error) {
	old, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return nil, err
	}
	raw := *old
	raw.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP |
		unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	raw.Oflag &^= unix.OPOST
	raw.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	raw.Cflag &^= unix.CSIZE | unix.PARENB
	raw.Cflag |= unix.CS8
	raw.Cc[unix.VMIN] = 1
	raw.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, &raw); err != nil {
		return nil, err
	}
	return old, nil
}

func ttyRestore(fd int, st any) {
	if t, ok := st.(*unix.Termios); ok {
		_ = unix.IoctlSetTermios(fd, unix.TCSETS, t)
	}
}

func ttySize(fd int) (rows, cols int) {
	ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	if err != nil {
		return 24, 80
	}
	if ws.Row < 1 {
		ws.Row = 24
	}
	if ws.Col < 1 {
		ws.Col = 80
	}
	return int(ws.Row), int(ws.Col)
}
