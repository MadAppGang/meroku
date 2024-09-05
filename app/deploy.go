package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/aymerick/raymond"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
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

func runCommandToDeploy(env string) error {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Println("Error getting current working directory:", err)
		os.Exit(1)
	}
	defer os.Chdir(wd)

	createFolderIfNotExists("env")
	err = createFolderIfNotExists(filepath.Join("env", env))
	if err != nil {
		fmt.Println("Error creating folder for environment:", err)
		os.Exit(1)
	}
	//
	applyTemplate(env)
	buildDeploymentLambda(env)

	e, err := loadEnv(env)
	if err != nil {
		fmt.Println("Error loading environment:", err)
		os.Exit(1)
	}
	checkBucketStateForEnv(e)

	err = os.Chdir(filepath.Join("env", env))
	if err != nil {
		fmt.Println("Error changing directory to env folder:", err)
		os.Exit(1)
	}
	terraformInitIfNeeded()
	return runTerrafromApply()

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

func applyTemplate(env string) {
	// Read the template file
	templateContent, err := os.ReadFile(filepath.Join("infrastructure", "env", "main.hbs"))
	if err != nil {
		fmt.Printf("error reading template file: %v", err)
		os.Exit(1)
	}

	envMap, err := loadEnvToMap(env + ".yaml")
	envMap["modules"] = "../../infrastructure/modules"
	if err != nil {
		fmt.Printf("error loading environment: %v", err)
		os.Exit(1)
	}
	// Create a new template and parse the content
	tmpl, err := raymond.Parse(string(templateContent))
	if err != nil {
		fmt.Printf("error parsing template: %v", err)
		os.Exit(1)
	}
	// Execute the template with the environment data
	result, err := tmpl.Exec(envMap)
	if err != nil {
		fmt.Printf("Error executing template: %+v\n", err)
		os.Exit(1)
	}

	os.WriteFile(filepath.Join("env", env, "main.tf"), []byte(result), 0o644)
}

func terraformInitIfNeeded() {
	if _, err := os.Stat(".terraform"); os.IsNotExist(err) {
		action := func() {
			cmd := exec.Command("terraform", "init")
			output, err := cmd.CombinedOutput()
			if err != nil {
				lines := strings.Split(string(output), "\n")
				for _, line := range lines {
					fmt.Println(strings.TrimSpace(line))
				}
			}
		}
		_ = spinner.New().Title("Initializing tarraform for your environment...").Action(action).Run()
		fmt.Println("✅ Terraform initialized successfully.")
		return
	} else if err != nil {
		fmt.Printf("Error checking .terraform directory: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Terraform already initialized.")
}

func runTerrafromApply() error {
	err := runCommandWithOutput("terraform", "plan")
	if err != nil {
		return err
	}

	fmt.Println("✅ Terraform plan completed successfully.")
	// Ask the user if they want to apply or return to the main menu
	result := false
	huh.NewConfirm().
		Title("Do you want to apply the Terraform changes?").
		Description("Select 'Yes' to apply, 'No' to return to the main menu.").
		Affirmative("Yes").
		Negative("No").
		Value(&result).
		Run()

	if !result {
		fmt.Println("Returning to main menu...")
		return nil
	}

	fmt.Println("Applying Terraform changes...")
	return runCommandWithOutput("terraform", "apply", "-auto-approve")
}

func buildDeploymentLambda(env string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("error getting current working directory: %w", err)
	}
	defer os.Chdir(wd)

	os.RemoveAll(filepath.Join("env", env, "ci_lambda.zip"))
	os.Chdir("infrastructure/modules/workloads/ci_lambda")
	os.RemoveAll("bootstrap")

	os.Setenv("GOOS", "linux")
	os.Setenv("GOARCH", "amd64")
	if err := runCommandWithOutput("go", "build", "-o", "bootstrap", "."); err != nil {
		return fmt.Errorf("error building deployment lambda: %w", err)
	}

	return nil
}
