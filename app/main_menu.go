package main

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/samber/lo"
)

// returns env to edit
func mainMenu() string {
	//
	envs, err := findFilesWithExts([]string{".yaml", ".yml"})
	if err != nil {
		panic(err)
	}
	options := lo.Map(envs, func(s string, _ int) huh.Option[string] {
		return huh.NewOption(fmt.Sprintf("Edit %s environment", s), fmt.Sprintf("env:%s", s))
	})
	options = append(options, huh.NewOption("Create new environment", "create"))
	options = append(options, huh.NewOption("Deploy environment", "deploy"))
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
	case action == "exit":
		os.Exit(0)
	}
	return ""
}

func createEnvMenu() string {
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

	e := createEnv(name)
	err := saveEnv(name, e)
	if err != nil {
		fmt.Println("Error saving environment:", err)
		os.Exit(1)
	}
	return name
}
