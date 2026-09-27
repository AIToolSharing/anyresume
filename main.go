// Command anyresume shows every Claude Code session on this computer in one
// list and resumes the chosen session in its own folder. It runs in any
// terminal and as a herdr plugin. It is not an Anthropic product.
package main

import (
	"os"

	"github.com/AIToolSharing/anyresume/internal/app"
)

// version is set by the release build.
var version = "dev"

func main() {
	os.Exit(app.Run(os.Args[1:], version))
}
