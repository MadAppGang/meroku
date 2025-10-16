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
			huh.NewNote().
				Title("Update Cancelled").
				Description("Infrastructure update was cancelled.").
				Run()
			return nil
		}
		_ = spinner.New().Title("Updating the infrastructure...").Action(initProject).Run()
		
		// Show success message after update
		huh.NewNote().
			Title("Update Complete").
			Description(fmt.Sprintf("Infrastructure updated successfully to version %s", remoteVersion)).
			Run()
	} else {
		// Show up-to-date message using huh
		huh.NewNote().
			Title("Already Up-to-Date").
			Description(fmt.Sprintf("Your infrastructure is already at the latest version (%s)", localVersion)).
			Run()
	}

	return nil
}

// VersionCheckResult holds the result of a version check
type VersionCheckResult struct {
	LocalVersion     string
	RemoteVersion    string
	UpdateAvailable  bool
	Error            error
}

// checkVersionAtStartup performs a silent version check without blocking UI
func checkVersionAtStartup() VersionCheckResult {
	result := VersionCheckResult{}

	// Fetch the remote version.txt file from GitHub repository
	resp, err := http.Get("https://raw.githubusercontent.com/MadAppGang/infrastructure/main/version.txt")
	if err != nil {
		result.Error = fmt.Errorf("failed to fetch remote version: %w", err)
		return result
	}
	defer resp.Body.Close()

	remoteVersionData, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = fmt.Errorf("failed to read remote version body: %w", err)
		return result
	}

	result.RemoteVersion = strings.TrimSpace(string(remoteVersionData))

	// Read the local version.txt file
	localVersionData, err := os.ReadFile("./infrastructure/version.txt")
	if err != nil {
		result.Error = fmt.Errorf("failed to read local version file: %w", err)
		return result
	}

	result.LocalVersion = strings.TrimSpace(string(localVersionData))

	// Compare versions using semver
	remoteVer, err := semver.ParseTolerant(result.RemoteVersion)
	if err != nil {
		result.Error = fmt.Errorf("failed to parse remote version: %w", err)
		return result
	}

	localVer, err := semver.ParseTolerant(result.LocalVersion)
	if err != nil {
		result.Error = fmt.Errorf("failed to parse local version: %w", err)
		return result
	}

	result.UpdateAvailable = remoteVer.GT(localVer)
	return result
}

// promptForUpdate prompts the user to update if a new version is available
func promptForUpdate(result VersionCheckResult) error {
	if !result.UpdateAvailable {
		return nil
	}

	confirm := false
	if err := huh.NewConfirm().
		Title("New infrastructure version available!").
		Description(fmt.Sprintf("Current version: %s, Available version: %s", result.LocalVersion, result.RemoteVersion)).
		Affirmative("Update Now").
		Negative("Skip").
		Value(&confirm).
		Run(); err != nil {
		return err
	}

	if !confirm {
		huh.NewNote().
			Title("Update Skipped").
			Description("You can update later by selecting 'Check for updates' from the menu.").
			Run()
		return nil
	}

	// Run the update
	_ = spinner.New().Title("Updating the infrastructure...").Action(initProject).Run()

	// Show success message after update
	huh.NewNote().
		Title("Update Complete").
		Description(fmt.Sprintf("Infrastructure updated successfully to version %s", result.RemoteVersion)).
		Run()

	return nil
}
