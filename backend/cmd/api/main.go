// Command api is the public HTTP process role.
//
// Configuration is validated once at startup via the platform config package;
// HTTP wiring lands in P01-T05. No business code lives here.
package main

import (
	"fmt"
	"os"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "anpfuel api:", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, "anpfuel api: running in "+string(cfg.Env)+" (HTTP wiring lands in P01-T05)")
}
