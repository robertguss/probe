//go:build hostile && unix && !linux && !darwin

package hostilefsx_test

import "fmt"

func realExclusiveRename(parentFd int, stageName, destName string) error {
	return fmt.Errorf("exclusive rename not implemented on this GOOS")
}
