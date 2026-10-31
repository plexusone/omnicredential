// Command omnicredential keeps a registry of credentials without their
// values, records their lifecycle and reports on them. It reads the
// registry and the event log as JSON files.
package main

import (
	"os"

	"github.com/plexusone/omnicredential/cli"
)

func main() {
	if err := cli.New(cli.Options{}).Execute(); err != nil {
		os.Exit(1)
	}
}
