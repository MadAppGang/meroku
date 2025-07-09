package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
)

func updateInfrastructure() error {
	// Fetch the remote version.txt file from GitHub repository
	resp, err := http.Get("https://raw.githubusercontent.com/MadAppGang/infrastructure/main/version.txt")
	if err != nil {
		return fmt.Errorf("failed to fetch remote version: %w", err)
	}
	defer resp.Body.Close()

	remoteVersionData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read remote version body: %w", err)
	}

	remoteVersion := strings.TrimSpace(string(remoteVersionData))

	// Read the local version.txt file
	localVersionData, err := os.ReadFile("./infrastructure/version.txt")
	if err != nil {
		return fmt.Errorf("failed to read local version file: %w", err)
	}

	localVersion := strings.TrimSpace(string(localVersionData))

	// Compare versions using semver
	remoteVer, err := semver.ParseTolerant(remoteVersion)
	if err != nil {
		return fmt.Errorf("failed to parse remote version: %w", err)
	}

	localVer, err := semver.ParseTolerant(localVersion)
	if err != nil {
		return fmt.Errorf("failed to parse local version: %w", err)
	}

	if remoteVer.GT(localVer) {
		confirm := false
		if err = huh.NewConfirm().
			Title("Do you want to update the infrastructure?").
			Description(fmt.Sprintf("Current version: %s, Remote version: %s", localVersion, remoteVersion)).
			Affirmative("Update").
			Negative("Cancel").
			Value(&confirm).
			Run(); err != nil {
			os.Exit(1)
		}
		if !confirm {
			fmt.Println("Update cancelled.")
			return nil
		}
		_ = spinner.New().Title("Updating the infrastructure...").Action(initProject).Run()
	} else {
		fmt.Println("👍 Local version is up-to-date with remote version.")
	}

	return nil
}
