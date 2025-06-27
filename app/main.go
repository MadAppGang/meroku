package main

import (
	"fmt"
	"log/slog"
	"os"
)

func main() {
	registerCustomHelpers()
	file, err := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o666)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	jsonHandler := slog.NewJSONHandler(file, nil)
	logger := slog.New(jsonHandler)
	slog.SetDefault(logger)

	err = selectAWSProfile()
	if err != nil {
		fmt.Println("Error selecting AWS profile:", err)
		os.Exit(1)
	}
	// --- Run main menu ---
	mainMenu()
	os.Exit(0)
}
