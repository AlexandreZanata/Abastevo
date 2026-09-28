// Command worker is the background-job process role.
//
// Configuration is validated once at startup via the platform config package;
// job dispatch lands in P03-T08. No business code lives here.
package main

import (
	"fmt"
	"os"

	"github.com/AlexandreZanata/brazil-fuel-prices/backend/internal/platform/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "anpfuel worker:", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, "anpfuel worker: running in "+string(cfg.Env)+" (dispatch lands in P03-T08)")
}
