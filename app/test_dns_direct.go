// +build ignore

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("Testing DNS status directly...")
	
	config, err := loadDNSConfig()
	if err != nil {
		fmt.Printf("Error loading DNS config: %v\n", err)
		os.Exit(1)
	}

	if config == nil {
		fmt.Println("No DNS configuration found.")
		fmt.Println("Run './meroku dns setup' to configure DNS.")
	} else {
		fmt.Printf("Found DNS config for domain: %s\n", config.RootDomain)
	}
}