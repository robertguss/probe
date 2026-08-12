//go:build darwin || freebsd || openbsd || netbsd || dragonfly

package e4

import "golang.org/x/sys/unix"

const termiosGet = unix.TIOCGETA
