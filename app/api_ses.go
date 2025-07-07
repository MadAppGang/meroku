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
	"github.com/aws/aws-sdk-go-v2/service/support"
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
		// Apply the same logic as in the Terraform template
		if envName == "prod" {
			fromEmail = fmt.Sprintf("noreply@mail.%s", envConfig.Domain.DomainName)
		} else {
			fromEmail = fmt.Sprintf("noreply@mail.%s.%s", envName, envConfig.Domain.DomainName)
		}
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

// ProductionAccessRequest represents the request to submit SES production access
type ProductionAccessRequest struct {
	WebsiteURL                string `json:"websiteUrl"`
	UseCaseDescription        string `json:"useCaseDescription"`
	MailingListBuildProcess   string `json:"mailingListBuildProcess"`
	BounceComplaintProcess    string `json:"bounceComplaintProcess"`
	AdditionalInfo            string `json:"additionalInfo"`
	ExpectedDailyVolume       string `json:"expectedDailyVolume"`
	ExpectedPeakVolume        string `json:"expectedPeakVolume"`
	ContactLanguage           string `json:"contactLanguage"` // "en" or other language codes
}

// ProductionAccessResponse represents the response after submitting production access request
type ProductionAccessResponse struct {
	Success  bool   `json:"success"`
	CaseID   string `json:"caseId,omitempty"`
	Error    string `json:"error,omitempty"`
	Message  string `json:"message,omitempty"`
}

// submitSESProductionAccess submits a request to move SES out of sandbox mode
func submitSESProductionAccess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get environment from query parameter
	envName := r.URL.Query().Get("env")
	if envName == "" {
		envName = "prod" // Default to prod for production access
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ProductionAccessResponse{
			Success: false,
			Error:   "Failed to read request body",
		})
		return
	}
	defer r.Body.Close()

	var req ProductionAccessRequest
	if err := json.Unmarshal(body, &req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ProductionAccessResponse{
			Success: false,
			Error:   fmt.Sprintf("Invalid request body: %v", err),
		})
		return
	}

	// Validate required fields
	if req.WebsiteURL == "" || req.UseCaseDescription == "" || req.MailingListBuildProcess == "" || req.BounceComplaintProcess == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ProductionAccessResponse{
			Success: false,
			Error:   "websiteUrl, useCaseDescription, mailingListBuildProcess, and bounceComplaintProcess are required",
		})
		return
	}

	// Load AWS configuration for us-east-1 (Support API only works in us-east-1)
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ProductionAccessResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to load AWS config: %v", err),
		})
		return
	}

	// Create Support client
	supportClient := support.NewFromConfig(cfg)

	// Build the case communication body
	communicationBody := fmt.Sprintf(`I would like to request production access for Amazon SES.

Website URL: %s

Use Case Description:
%s

How will you build or acquire your mailing list?
%s

How will you handle bounces and complaints?
%s

Expected Daily Volume: %s
Expected Peak Volume: %s

Additional Information:
%s

Please review and approve our request to move out of the SES sandbox mode.`,
		req.WebsiteURL,
		req.UseCaseDescription,
		req.MailingListBuildProcess,
		req.BounceComplaintProcess,
		req.ExpectedDailyVolume,
		req.ExpectedPeakVolume,
		req.AdditionalInfo)

	// Set default language if not provided
	if req.ContactLanguage == "" {
		req.ContactLanguage = "en"
	}

	// Create the support case
	createCaseInput := &support.CreateCaseInput{
		Subject:             aws.String("Request to increase SES sending limits"),
		ServiceCode:         aws.String("amazon-ses"),
		SeverityCode:        aws.String("low"),
		CategoryCode:        aws.String("sending-limits-increase"),
		CommunicationBody:   aws.String(communicationBody),
		Language:            aws.String(req.ContactLanguage),
		IssueType:           aws.String("technical"),
	}

	result, err := supportClient.CreateCase(ctx, createCaseInput)
	if err != nil {
		// Check if it's a subscription error
		if err.Error() == "SubscriptionRequiredException" || err.Error() == "operation error Support: CreateCase, https response error StatusCode: 403" {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(ProductionAccessResponse{
				Success: false,
				Error:   "AWS Support subscription required. Please ensure you have a Business, Enterprise On-Ramp, or Enterprise support plan.",
				Message: "To request SES production access, you need an AWS Support plan. Alternatively, you can submit the request through the AWS Console.",
			})
			return
		}
		
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ProductionAccessResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to create support case: %v", err),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ProductionAccessResponse{
		Success: true,
		CaseID:  *result.CaseId,
		Message: "Production access request submitted successfully. AWS will review your request and respond within 24-48 hours.",
	})
}

// ProductionAccessPrefillResponse represents prefilled data for production access request
type ProductionAccessPrefillResponse struct {
	WebsiteURL                string `json:"websiteUrl"`
	UseCaseDescription        string `json:"useCaseDescription"`
	MailingListBuildProcess   string `json:"mailingListBuildProcess"`
	BounceComplaintProcess    string `json:"bounceComplaintProcess"`
	AdditionalInfo            string `json:"additionalInfo"`
	ExpectedDailyVolume       string `json:"expectedDailyVolume"`
	ExpectedPeakVolume        string `json:"expectedPeakVolume"`
	DomainName                string `json:"domainName"`
}

// getProductionAccessPrefill returns prefilled data for SES production access request
func getProductionAccessPrefill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get environment from query parameter
	envName := r.URL.Query().Get("env")
	if envName == "" {
		envName = "prod"
	}

	// Load environment config to get domain info
	filename := fmt.Sprintf("%s.yaml", envName)
	content, err := os.ReadFile(filename)
	if err != nil {
		// Return generic prefill if config not found
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ProductionAccessPrefillResponse{
			WebsiteURL:              "https://example.com",
			UseCaseDescription:      "We are building a web application that needs to send transactional emails to our users. These emails include account verification, password resets, order confirmations, and important notifications about their account activity.",
			MailingListBuildProcess: "Users explicitly sign up on our website and provide consent to receive emails. We use double opt-in verification to ensure email addresses are valid and that users want to receive our communications. We never purchase email lists or add users without their explicit consent.",
			BounceComplaintProcess:  "We have implemented automated bounce and complaint handling using AWS SNS topics. Hard bounces are immediately removed from our mailing list. Soft bounces are retried up to 3 times before removal. Complaints result in immediate unsubscription. We maintain a suppression list to prevent sending to addresses that have bounced or complained.",
			AdditionalInfo:          "We follow email best practices including: proper authentication (SPF, DKIM, DMARC), clear unsubscribe links in every email, sending only to users who have explicitly opted in, and monitoring our sender reputation. We use AWS CloudWatch to track bounce and complaint rates.",
			ExpectedDailyVolume:     "1000-5000",
			ExpectedPeakVolume:      "10000",
			DomainName:              "example.com",
		})
		return
	}

	var envConfig Env
	if err := yaml.Unmarshal(content, &envConfig); err != nil {
		// Return generic prefill if parsing fails
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ProductionAccessPrefillResponse{
			WebsiteURL:              "https://example.com",
			UseCaseDescription:      "We are building a web application that needs to send transactional emails to our users. These emails include account verification, password resets, order confirmations, and important notifications about their account activity.",
			MailingListBuildProcess: "Users explicitly sign up on our website and provide consent to receive emails. We use double opt-in verification to ensure email addresses are valid and that users want to receive our communications. We never purchase email lists or add users without their explicit consent.",
			BounceComplaintProcess:  "We have implemented automated bounce and complaint handling using AWS SNS topics. Hard bounces are immediately removed from our mailing list. Soft bounces are retried up to 3 times before removal. Complaints result in immediate unsubscription. We maintain a suppression list to prevent sending to addresses that have bounced or complained.",
			AdditionalInfo:          "We follow email best practices including: proper authentication (SPF, DKIM, DMARC), clear unsubscribe links in every email, sending only to users who have explicitly opted in, and monitoring our sender reputation. We use AWS CloudWatch to track bounce and complaint rates.",
			ExpectedDailyVolume:     "1000-5000",
			ExpectedPeakVolume:      "10000",
			DomainName:              "example.com",
		})
		return
	}

	// Determine domain name
	domainName := ""
	if envConfig.Ses.Enabled && envConfig.Ses.DomainName != "" {
		domainName = envConfig.Ses.DomainName
	} else if envConfig.Domain.Enabled && envConfig.Domain.DomainName != "" {
		domainName = envConfig.Domain.DomainName
	}

	// Create project-specific prefilled data
	projectName := envConfig.Project
	websiteURL := fmt.Sprintf("https://%s", domainName)
	
	response := ProductionAccessPrefillResponse{
		WebsiteURL: websiteURL,
		UseCaseDescription: fmt.Sprintf(
			"We are operating %s, a production web application that requires email capabilities for essential user communications. "+
			"Our application sends transactional emails including:\n"+
			"- User registration confirmations and email verification\n"+
			"- Password reset requests\n"+
			"- Important account notifications and security alerts\n"+
			"- Service updates and system notifications\n"+
			"- Transaction confirmations and receipts\n\n"+
			"All emails are triggered by user actions or system events and are essential for the operation of our service.",
			projectName),
		
		MailingListBuildProcess: fmt.Sprintf(
			"Our email list is built exclusively through organic user registration on %s. "+
			"Our process ensures compliance and user consent:\n\n"+
			"1. Users voluntarily sign up on our platform at %s\n"+
			"2. We implement double opt-in email verification for all new registrations\n"+
			"3. Users must explicitly confirm their email address before receiving any communications\n"+
			"4. We provide clear privacy policy and terms of service during registration\n"+
			"5. Users can manage their email preferences in their account settings\n"+
			"6. We never purchase, rent, or acquire email lists from third parties\n"+
			"7. All user data is stored securely in compliance with data protection regulations",
			websiteURL, websiteURL),
		
		BounceComplaintProcess: fmt.Sprintf(
			"We have implemented a comprehensive bounce and complaint handling system for %s:\n\n"+
			"**Automated Handling:**\n"+
			"- AWS SNS topics configured for bounce and complaint notifications\n"+
			"- Real-time processing of bounce and complaint events\n"+
			"- Automatic suppression list management\n\n"+
			"**Bounce Management:**\n"+
			"- Hard bounces: Immediately added to suppression list\n"+
			"- Soft bounces: Retried up to 3 times over 24 hours before suppression\n"+
			"- Bounce rate monitoring with alerts if rate exceeds 5%%\n\n"+
			"**Complaint Management:**\n"+
			"- Complaints result in immediate unsubscription\n"+
			"- User is added to permanent suppression list\n"+
			"- Manual review process for complaint patterns\n"+
			"- Complaint rate monitoring with alerts if rate exceeds 0.1%%\n\n"+
			"**Additional Measures:**\n"+
			"- Weekly reports on email metrics\n"+
			"- Suppression list is checked before every send\n"+
			"- Re-engagement campaigns for inactive users\n"+
			"- List hygiene performed quarterly",
			projectName),
		
		AdditionalInfo: fmt.Sprintf(
			"**Email Infrastructure for %s:**\n\n"+
			"**Authentication & Security:**\n"+
			"- SPF records properly configured for %s\n"+
			"- DKIM signing enabled for all outbound emails\n"+
			"- DMARC policy implemented with monitoring\n"+
			"- TLS encryption for email transmission\n\n"+
			"**Best Practices Implementation:**\n"+
			"- Clear and visible unsubscribe links in every email\n"+
			"- Consistent 'From' address: noreply@%s\n"+
			"- Descriptive subject lines without misleading content\n"+
			"- Plain text alternatives for all HTML emails\n"+
			"- Mobile-responsive email templates\n\n"+
			"**Monitoring & Compliance:**\n"+
			"- AWS CloudWatch dashboards for email metrics\n"+
			"- Automated alerts for bounce/complaint thresholds\n"+
			"- Regular sender reputation monitoring\n"+
			"- GDPR and CAN-SPAM compliance measures\n"+
			"- Data retention policies in place\n\n"+
			"**Technical Implementation:**\n"+
			"- Email sending through AWS SDK with proper error handling\n"+
			"- Rate limiting to respect AWS SES quotas\n"+
			"- Retry logic with exponential backoff\n"+
			"- Comprehensive logging for audit trails",
			projectName, domainName, domainName),
		
		ExpectedDailyVolume: "1000-5000",
		ExpectedPeakVolume:  "10000",
		DomainName:          domainName,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}