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

	// --- Run main menu ---
	env := mainMenu()

	e, err := loadEnv(env)
	if err != nil {
		fmt.Println("Error loading environment:", err)
		os.Exit(1)
	}
	RunEnvEdit(e)
}
