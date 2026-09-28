// Command api is the public HTTP process role.
//
// P01-T02 only establishes a compilable composition root. HTTP wiring lands
// in P01-T05; no business code lives here.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stdout, "anpfuel api: composition root not wired yet (P01-T05)")
}
