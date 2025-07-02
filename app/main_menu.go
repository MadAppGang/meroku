package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
)

// returns env to edit
func mainMenu() string {
	// check if project is init
	initProjectIfNeeded()

	options := []huh.Option[string]{huh.NewOption("Create new environment", "create")}
	options = append(options, huh.NewOption("Deploy environment", "deploy"))
	options = append(options, huh.NewOption("Check for updates", "update"))
	options = append(options, huh.NewOption("Open web app", "api"))
	options = append(options, huh.NewOption("Exit", "exit"))

	action := ""

	huh.NewSelect[string]().
		Title("Select an action.").
		Options(
			options...,
		).
		Value(&action).
		Run()

	switch {
	case strings.HasPrefix(action, "env:"):
		return strings.TrimPrefix(action, "env:")
	case action == "create":
		return createEnvMenu()
	case action == "deploy":
		deployMenu()
		return mainMenu()
	case action == "update":
		err := updateInfrastructure()
		if err != nil {
			fmt.Println("Error updating infrastructure:", err)
			os.Exit(1)
		}
		return mainMenu()
	case action == "api":
		startSPAServer("8080")
		return mainMenu()
	case action == "exit":
		os.Exit(0)
	}
	return ""
}

func createEnvMenu() string {
	projectName := getProjectName()

	var name string
	huh.NewInput().
		Title("What is the name of the environment?").
		Value(&name).
		Run() // this is blocking.

	r := regexp.MustCompile(`^[a-z]{2,}$`)
	if !r.MatchString(name) {
		fmt.Println("Invalid environment name")
		fmt.Println("minimum 2 characters, all lowercases, only letters from a-z")
		return createEnvMenu()
	}

	envs, _ := findFilesWithExts([]string{".yaml", ".yml"})
	if slices.Contains(envs, name+".yaml") || slices.Contains(envs, name+".yml") {
		fmt.Println("Environment already exists, try another name")
		return createEnvMenu()
	}

	e := createEnv(projectName, name)
	err := saveEnv(e)
	if err != nil {
		fmt.Println("Error saving environment:", err)
		os.Exit(1)
	}
	return name
}

// we are getting the project name from the first yaml file in the current directory
// if there is no yaml file, we are asking user to input the project name
func getProjectName() string {
	envs, _ := findFilesWithExts([]string{".yaml", ".yml"})
	if len(envs) > 0 {
		e, err := loadEnv(envs[0])
		if err == nil {
			return e.Project
		}
	}

	var name string
	huh.NewInput().
		Title("What is the proejct name?").
		Value(&name).
		Run() // this is blocking.

	return name
}

func initProject() {
	os.RemoveAll("./infrastructure/")
	cmd := exec.Command("git", "clone", "--depth=1", "--branch=main", "https://github.com/MadAppGang/infrastructure.git", "./infrastructure")
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Println("Error cloning infrastructure:", output)
		os.Exit(1)
	}
	os.RemoveAll("./infrastructure/.git")
}

func initProjectIfNeeded() {
	if _, err := os.Stat("infrastructure"); os.IsNotExist(err) {
		answer := false
		huh.NewConfirm().
			Title("The project is not initialized, do you want to initialize it?").
			Affirmative("Yes 🚀").
			Negative("No 🤷‍♂️").
			Value(&answer).
			Run()
		if !answer {
			fmt.Println("Aborting, 👋!")
			os.Exit(1)
		}

		initProject()

		_ = spinner.New().Title("Initializing the project...").Action(initProject).Run()
	}
}
