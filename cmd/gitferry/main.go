package main

import (
	"fmt"
	"os"

	"github.com/yi-nology/git-ferry/internal/cli/commands"
)

// Version 由 make 构建时 -ldflags 注入。
var Version = "dev"

func main() {
	commands.Version = Version
	if err := commands.NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
