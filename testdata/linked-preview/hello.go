// ABOUTME: Provides a small Go source fixture for highlighting and copying.
// ABOUTME: Previewing this file displays its source without executing it.
package main

import "fmt"

func main() {
	for _, name := range []string{"Org", "Markdown", "DOCX"} {
		fmt.Printf("Hello, %s!\n", name)
	}
}
