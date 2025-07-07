package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ses"
	"github.com/aws/aws-sdk-go-v2/service/ses/types"
	"gopkg.in/yaml.v2"
)

// SESStatusResponse represents the SES account status
type SESStatusResponse struct {
	InSandbox           bool     `json:"inSandbox"`
	SendingEnabled      bool     `json:"sendingEnabled"`
	DailyQuota          int64    `json:"dailyQuota"`
	MaxSendRate         float64  `json:"maxSendRate"`
	SentLast24Hours     int64    `json:"sentLast24Hours"`
	VerifiedDomains     []string `json:"verifiedDomains"`
	VerifiedEmails      []string `json:"verifiedEmails"`
	SuppressionList     bool     `json:"suppressionListEnabled"`
	ReputationStatus    string   `json:"reputationStatus"`
	Region              string   `json:"region"`
}

// getSESStatus checks if SES is in sandbox mode and returns account status
func getSESStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Load AWS configuration
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("Failed to load AWS config: %v", err)})
		return
	}

	// Create SES client
	sesClient := ses.NewFromConfig(cfg)

	response := SESStatusResponse{
		Region: cfg.Region,
	}

	// Get sending quota (this tells us if we're in sandbox)
	quotaInput := &ses.GetSendQuotaInput{}
	quotaResult, err := sesClient.GetSendQuota(ctx, quotaInput)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("Failed to get SES quota: %v", err)})
		return
	}

	// Check if in sandbox - sandbox accounts have a daily quota of 200
	response.InSandbox = quotaResult.Max24HourSend <= 200
	response.DailyQuota = int64(quotaResult.Max24HourSend)
	response.MaxSendRate = quotaResult.MaxSendRate
	response.SentLast24Hours = int64(quotaResult.SentLast24Hours)

	// Check if sending is enabled
	enabledInput := &ses.GetAccountSendingEnabledInput{}
	enabledResult, err := sesClient.GetAccountSendingEnabled(ctx, enabledInput)
	if err == nil {
		response.SendingEnabled = enabledResult.Enabled
	}

	// Get verified domains
	domainsInput := &ses.ListVerifiedEmailAddressesInput{}
	domainsResult, err := sesClient.ListVerifiedEmailAddresses(ctx, domainsInput)
	if err == nil {
		response.VerifiedEmails = domainsResult.VerifiedEmailAddresses
	}

	// Get domain identities and their verification status
	identitiesInput := &ses.ListIdentitiesInput{
		IdentityType: types.IdentityTypeDomain,
		MaxItems:     aws.Int32(100),
	}
	identitiesResult, err := sesClient.ListIdentities(ctx, identitiesInput)
	if err == nil && len(identitiesResult.Identities) > 0 {
		// Get verification attributes for the identities
		verifyInput := &ses.GetIdentityVerificationAttributesInput{
			Identities: identitiesResult.Identities,
		}
		verifyResult, err := sesClient.GetIdentityVerificationAttributes(ctx, verifyInput)
		if err == nil {
			// Only include domains that are actually verified
			verifiedDomains := []string{}
			for domain, attrs := range verifyResult.VerificationAttributes {
				if attrs.VerificationStatus == types.VerificationStatusSuccess {
					verifiedDomains = append(verifiedDomains, domain)
				}
			}
			response.VerifiedDomains = verifiedDomains
		} else {
			response.VerifiedDomains = identitiesResult.Identities
		}
	}

	// Set default values for fields we can't easily query
	response.SuppressionList = false
	response.ReputationStatus = "Default"

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// getSESSandboxInfo provides information about SES sandbox limitations
func getSESSandboxInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sandboxInfo := map[string]interface{}{
		"limitations": []string{
			"Can only send to verified email addresses",
			"Maximum of 200 emails per 24-hour period",
			"Maximum send rate of 1 email per second",
			"Cannot send to unverified recipients",
		},
		"howToExit": []string{
			"Go to AWS Support Center",
			"Create a new case for 'Service limit increase'",
			"Choose 'SES Sending Limits' as the limit type",
			"Provide use case details and expected volume",
			"Wait for AWS approval (usually 24-48 hours)",
		},
		"requiredInfo": []string{
			"How you plan to build or acquire your mailing list",
			"How you plan to handle bounces and complaints",
			"Types of emails you plan to send",
			"How you will ensure recipients want your emails",
		},
		"tips": []string{
			"Set up bounce and complaint handling",
			"Configure DKIM and SPF records",
			"Have a clear unsubscribe process",
			"Start with low volume and gradually increase",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sandboxInfo)
}

// TestEmailRequest represents the request to send a test email
type TestEmailRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// TestEmailResponse represents the response after sending a test email
type TestEmailResponse struct {
	Success   bool   `json:"success"`
	MessageID string `json:"messageId,omitempty"`
	Error     string `json:"error,omitempty"`
}

// sendTestEmail sends a test email using SES
func sendTestEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TestEmailResponse{
			Success: false,
			Error:   "Failed to read request body",
		})
		return
	}
	defer r.Body.Close()

	var req TestEmailRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TestEmailResponse{
			Success: false,
			Error:   fmt.Sprintf("Invalid request body: %v", err),
		})
		return
	}

	// Validate required fields
	if req.To == "" || req.Subject == "" || req.Body == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(TestEmailResponse{
			Success: false,
			Error:   "to, subject, and body are required",
		})
		return
	}

	// Get environment from query parameter
	envName := r.URL.Query().Get("env")
	if envName == "" {
		envName = "dev" // Default to dev
	}

	// Load environment config to get the from email
	filename := fmt.Sprintf("%s.yaml", envName)
	content, err := os.ReadFile(filename)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(TestEmailResponse{
			Success: false,
			Error:   "Failed to load environment config",
		})
		return
	}

	var envConfig Env
	if err := yaml.Unmarshal(content, &envConfig); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(TestEmailResponse{
			Success: false,
			Error:   "Failed to parse environment config",
		})
		return
	}

	// Load AWS configuration first to check verified addresses
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(TestEmailResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to load AWS config: %v", err),
		})
		return
	}

	// Create SES client
	sesClient := ses.NewFromConfig(cfg)

	// Get verified email addresses first
	verifiedEmails := []string{}
	emailsInput := &ses.ListVerifiedEmailAddressesInput{}
	emailsResult, err := sesClient.ListVerifiedEmailAddresses(ctx, emailsInput)
	if err == nil && len(emailsResult.VerifiedEmailAddresses) > 0 {
		verifiedEmails = emailsResult.VerifiedEmailAddresses
	}

	// Determine the from email address
	fromEmail := "noreply@example.com" // Default
	
	// First, try to use a verified email if available
	if len(verifiedEmails) > 0 {
		fromEmail = verifiedEmails[0] // Use the first verified email
	} else if envConfig.Ses.Enabled && envConfig.Ses.DomainName != "" {
		fromEmail = fmt.Sprintf("noreply@%s", envConfig.Ses.DomainName)
	} else if envConfig.Domain.Enabled && envConfig.Domain.DomainName != "" {
		fromEmail = fmt.Sprintf("noreply@mail.%s", envConfig.Domain.DomainName)
	}

	// Create the email input
	input := &ses.SendEmailInput{
		Source: aws.String(fromEmail),
		Destination: &types.Destination{
			ToAddresses: []string{req.To},
		},
		Message: &types.Message{
			Subject: &types.Content{
				Data:    aws.String(req.Subject),
				Charset: aws.String("UTF-8"),
			},
			Body: &types.Body{
				Text: &types.Content{
					Data:    aws.String(req.Body),
					Charset: aws.String("UTF-8"),
				},
				Html: &types.Content{
					Data:    aws.String(fmt.Sprintf("<html><body><p>%s</p></body></html>", req.Body)),
					Charset: aws.String("UTF-8"),
				},
			},
		},
	}

	// Log the from email for debugging
	fmt.Printf("Sending test email from: %s to: %s\n", fromEmail, req.To)
	
	// Send the email
	result, err := sesClient.SendEmail(ctx, input)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(TestEmailResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to send email from %s: %v", fromEmail, err),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(TestEmailResponse{
		Success:   true,
		MessageID: *result.MessageId,
	})
}