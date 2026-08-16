//go:build !unix

package console

import "errors"

func ttyIsTerminal(fd int) bool { return false }

func ttyMakeRaw(fd int) (any, error) {
	return nil, errors.New("console UI not supported on this platform")
}

func ttyRestore(fd int, st any) {}

func ttySize(fd int) (rows, cols int) { return 24, 80 }
