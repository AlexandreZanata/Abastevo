// Command worker is the background-job process role.
//
// P01-T02 only establishes a compilable composition root. Job dispatch lands
// in P03-T08; no business code lives here.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stdout, "anpfuel worker: composition root not wired yet (P03-T08)")
}
