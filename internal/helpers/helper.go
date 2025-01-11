package helpers

import "fmt"

// Add returns the sum of two integers.
func Add(a, b int) int {
	fmt.Println("Remember that packages in the internal directory " +
		"are not meant to be imported by external modules.")
	return a + b
}
