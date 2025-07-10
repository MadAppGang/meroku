package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
)

var (
	profileFlag = flag.String("profile", "", "AWS profile to use (skips profile selection)")
	webFlag     = flag.Bool("web", false, "Open web app immediately")
)

func main() {
	// Parse command line flags
	flag.Parse()

	registerCustomHelpers()
	file, err := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	jsonHandler := slog.NewJSONHandler(file, nil)
	logger := slog.New(jsonHandler)
	slog.SetDefault(logger)

	// Handle profile selection
	if *profileFlag != "" {
		// Use the provided profile directly
		selectedAWSProfile = *profileFlag
		fmt.Printf("Using AWS profile: %s\n", selectedAWSProfile)
	} else {
		// Interactive profile selection
		err = selectAWSProfile()
		if err != nil {
			fmt.Println("Error selecting AWS profile:", err)
			os.Exit(1)
		}
	}

	// If --web flag is set, open web app directly
	if *webFlag {
		startSPAServerWithAutoOpen("8080", true, false)
		// Keep the program running
		fmt.Println("\nWeb server is running. Press Ctrl+C to stop.")
		select {}
	} else {
		// Run normal interactive menu
		mainMenu()
	}
	
	os.Exit(0)
}
