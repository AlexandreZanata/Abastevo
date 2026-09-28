// Command migrate is the restricted one-shot schema-migration tool.
//
// Configuration is validated once at startup via the platform config package;
// the migration runner lands in P01-T08. The API role must never run DDL.
package main

import (
	"fmt"
	"os"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "anpfuel migrate:", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, "anpfuel migrate: running in "+string(cfg.Env)+" (runner lands in P01-T08)")
}
