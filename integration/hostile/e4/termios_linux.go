//go:build linux

package e4

import "golang.org/x/sys/unix"

const termiosGet = unix.TCGETS
