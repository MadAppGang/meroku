package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/huh"
)

// AskChoice presents user with options using huh.Select
func AskChoice(question string, options []string) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("no options provided")
	}

	// Convert strings to huh.Option
	huhOptions := make([]huh.Option[string], len(options))
	for i, opt := range options {
		huhOptions[i] = huh.NewOption(opt, opt)
	}

	var selected string
	err := huh.NewSelect[string]().
		Title(question).
		Options(huhOptions...).
		Value(&selected).
		Run()

	if err != nil {
		return "", fmt.Errorf("selection cancelled: %w", err)
	}

	return selected, nil
}

// AskConfirm asks user yes/no question
func AskConfirm(question string) (bool, error) {
	var confirmed bool
	err := huh.NewConfirm().
		Title(question).
		Value(&confirmed).
		Run()

	if err != nil {
		return false, fmt.Errorf("confirmation cancelled: %w", err)
	}

	return confirmed, nil
}

// AskInput asks user for text input with validation
func AskInput(question, placeholder, validatorType string) (string, error) {
	var input string

	inputField := huh.NewInput().
		Title(question).
		Placeholder(placeholder).
		Value(&input)

	// Add validator based on type
	switch validatorType {
	case "url":
		inputField = inputField.Validate(ValidateURL)
	case "region":
		inputField = inputField.Validate(ValidateRegion)
	case "account_id":
		inputField = inputField.Validate(ValidateAccountID)
	case "role_name":
		inputField = inputField.Validate(ValidateRoleName)
	case "none":
		// No validation
	default:
		return "", fmt.Errorf("unknown validator type: %s", validatorType)
	}

	err := inputField.Run()
	if err != nil {
		return "", fmt.Errorf("input cancelled: %w", err)
	}

	return input, nil
}

// Validators

func ValidateURL(s string) error {
	if s == "" {
		return fmt.Errorf("URL cannot be empty")
	}
	if !strings.HasPrefix(s, "https://") {
		return fmt.Errorf("URL must start with https://")
	}
	if !strings.Contains(s, ".awsapps.com") && !strings.Contains(s, ".aws.amazon.com") {
		return fmt.Errorf("URL should be an AWS SSO portal URL")
	}
	return nil
}

func ValidateRegion(s string) error {
	if s == "" {
		return fmt.Errorf("region cannot be empty")
	}
	// Pattern: us-east-1, eu-west-2, ap-southeast-1
	regionRegex := regexp.MustCompile(`^[a-z]{2}-[a-z]+-\d+$`)
	if !regionRegex.MatchString(s) {
		return fmt.Errorf("invalid region format (expected: us-east-1)")
	}
	return nil
}

func ValidateAccountID(s string) error {
	if s == "" {
		return fmt.Errorf("account ID cannot be empty")
	}
	if len(s) != 12 {
		return fmt.Errorf("account ID must be exactly 12 digits")
	}
	accountRegex := regexp.MustCompile(`^\d{12}$`)
	if !accountRegex.MatchString(s) {
		return fmt.Errorf("account ID must be numeric (12 digits)")
	}
	return nil
}

func ValidateRoleName(s string) error {
	if s == "" {
		return fmt.Errorf("role name cannot be empty")
	}
	// Valid characters: alphanumeric, plus, equals, comma, period, at, underscore, hyphen
	roleRegex := regexp.MustCompile(`^[\w+=,.@-]+$`)
	if !roleRegex.MatchString(s) {
		return fmt.Errorf("invalid role name (allowed: alphanumeric, +, =, ., @, _, -)")
	}
	return nil
}
