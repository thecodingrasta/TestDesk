//go:build !windows

package lifecycle

import (
	"os"
	"os/exec"
)

func attachStartupHandles(cmd *exec.Cmd, read, write *os.File) ([]*os.File, uint64, uint64, error) {
	first := 3 + len(cmd.ExtraFiles)
	cmd.ExtraFiles = append(cmd.ExtraFiles, read, write)
	return []*os.File{read, write}, uint64(first), uint64(first + 1), nil
}
