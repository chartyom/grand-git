package main

import (
	"fmt"
	"os"

	"github.com/chartyom/grand-git/internal/gg"
)

func main() {
	if err := gg.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "gg:", err)
		os.Exit(1)
	}
}
