package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/charmbracelet/huh"
	"github.com/samber/lo"
)

func deployMenu() {
	var env string

	envs, err := findFilesWithExts([]string{".yaml", ".yml"})
	if err != nil {
		panic(err)
	}
	options := lo.Map(envs, func(s string, _ int) huh.Option[string] {
		return huh.NewOption(fmt.Sprintf("Deploy %s environment", s), s)
	})
	options = append(options, huh.NewOption("Back to main menu", "go:back"))

	huh.NewSelect[string]().
		Title("Select an environment to deploy").
		Options(
			options...,
		).
		Value(&env).
		Run()

	if env == "go:back" {
		return
	} else if env == "" {
		fmt.Println("No environment selected")
		os.Exit(1)
	}
	runCommandToDeploy(env)
	deployMenu()
}

func runCommandToDeploy(env string) {
	createFolderIfNotExists("env")
	err := createFolderIfNotExists(filepath.Join("env", env))
	if err != nil {
		fmt.Println("Error creating folder for environment:", err)
		os.Exit(1)
	}
	//
	applyTemplate()
	err = os.Chdir(filepath.Join("env", env))
	if err != nil {
		fmt.Println("Error changing directory to env folder:", err)
		os.Exit(1)
	}

	cmd := exec.Command("terraform", "apply")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Println("Error creating stdout pipe:", err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Println("Error creating stderr pipe:", err)
		return
	}

	// Start the command
	err = cmd.Start()
	if err != nil {
		fmt.Println("Error starting command:", err)
		return
	}

	// Create channels to signal when we're done reading from stdout and stderr
	doneChan := make(chan bool, 2)

	// Start goroutine to read from stdout
	go streamOutput(stdout, "STDOUT", doneChan)

	// Start goroutine to read from stderr
	go streamOutput(stderr, "STDERR", doneChan)

	// Wait for both stdout and stderr to finish
	for i := 0; i < 2; i++ {
		<-doneChan
	}

	// Wait for the command to finish
	err = cmd.Wait()
	if err != nil {
		fmt.Println("Command finished with error:", err)
	}
	os.Chdir(filepath.Join("..", ".."))
}

func streamOutput(r io.Reader, prefix string, doneChan chan bool) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fmt.Printf("%s: %s\n", prefix, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		fmt.Printf("%s: Error reading output: %s\n", prefix, err)
	}
	doneChan <- true
}

func applyTemplate() {
}
