package gg

import (
	"errors"
	"os"
)

// Copying remains independent on macOS, including filesystems without cloning.
func cloneFile(dest, source *os.File) error { return errors.New("file cloning is unavailable") }
