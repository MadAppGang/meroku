package main

import (
	"fmt"
	"os/exec"
)

func runCommandWithOutput(name string, args ...string) error {
	cmd := exec.Command(name, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Println("Error creating stdout pipe:", err)
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		fmt.Println("Error creating stderr pipe:", err)
		return err
	}

	// Start the command
	err = cmd.Start()
	if err != nil {
		fmt.Println("Error starting command:", err)
		return err
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
		return err
	}
	return nil
}
