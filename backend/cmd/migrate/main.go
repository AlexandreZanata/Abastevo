// Command migrate is the restricted one-shot schema-migration tool.
//
// P01-T02 only establishes a compilable composition root. The migration
// runner lands in P01-T08; the API role must never run DDL.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stdout, "anpfuel migrate: migration runner not wired yet (P01-T08)")
}
