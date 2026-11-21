// Command cfm is the command line client of the cfm-sendbox execution engine.
package main

import (
	"os"

	"github.com/Coding-for-Machine/cfm-sendbox/internal/cli/commands"
)

func main() {
	os.Exit(commands.Run(os.Args[1:], commands.NewEnv()))
}
