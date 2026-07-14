// Command xenolist inventories every piece of external code a repository
// executes — GitHub Actions, container base images, curl|bash installers,
// package runners like npx and go run — and produces one census report.
package main

import (
	"os"

	"github.com/JaydenCJ/xenolist/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
