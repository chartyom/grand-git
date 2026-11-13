package gg

import (
	"golang.org/x/sys/unix"
	"os"
)

func cloneFile(dest, source *os.File) error {
	return unix.IoctlFileClone(int(dest.Fd()), int(source.Fd()))
}
