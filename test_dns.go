package main

import (
	"fmt"
	"os"
)

func main() {
	// Test DNS status directly
	err := runDNSStatus(nil, []string{})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}