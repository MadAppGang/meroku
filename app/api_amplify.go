package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/amplify"
	"github.com/aws/aws-sdk-go-v2/service/amplify/types"
)

type AmplifyAppInfo struct {
	AppID             string                    `json:"appId"`
	Name              string                    `json:"name"`
	DefaultDomain     string                    `json:"defaultDomain"`
	CustomDomain      string                    `json:"customDomain,omitempty"`
	Repository        string                    `json:"repository"`
	CreateTime        time.Time                 `json:"createTime"`
	LastUpdateTime    time.Time                 `json:"lastUpdateTime"`
	Branches          []AmplifyBranchInfo       `json:"branches"`
}

type AmplifyBranchInfo struct {
	BranchName        string                    `json:"branchName"`
	Stage             string                    `json:"stage"`
	DisplayName       string                    `json:"displayName"`
	EnableAutoBuild   bool                      `json:"enableAutoBuild"`
	EnablePullRequestPreview bool               `json:"enablePullRequestPreview"`
	BranchURL         string                    `json:"branchUrl"`
	LastBuildStatus   string                    `json:"lastBuildStatus,omitempty"`
	LastBuildTime     *time.Time                `json:"lastBuildTime,omitempty"`
	LastBuildDuration int32                     `json:"lastBuildDuration,omitempty"`
	LastCommitId      string                    `json:"lastCommitId,omitempty"`
	LastCommitMessage string                    `json:"lastCommitMessage,omitempty"`
	LastCommitTime    *time.Time                `json:"lastCommitTime,omitempty"`
	CreateTime        time.Time                 `json:"createTime"`
	UpdateTime        time.Time                 `json:"updateTime"`
}

type AmplifyAppsResponse struct {
	Apps []AmplifyAppInfo `json:"apps"`
}

// getAmplifyApps handles GET /api/amplify/apps
func getAmplifyApps(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get environment from query parameter
	env := r.URL.Query().Get("environment")
	if env == "" {
		http.Error(w, "Environment parameter is required", http.StatusBadRequest)
		return
	}

	// Get profile from query parameter
	profile := r.URL.Query().Get("profile")
	
	// Create AWS config
	ctx := context.Background()
	cfg, err := createAWSConfig(ctx, profile)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create AWS config: %v", err), http.StatusInternalServerError)
		return
	}

	// Create Amplify client
	amplifyClient := amplify.NewFromConfig(cfg)

	// List all Amplify apps
	listAppsResp, err := amplifyClient.ListApps(ctx, &amplify.ListAppsInput{})
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list Amplify apps: %v", err), http.StatusInternalServerError)
		return
	}

	// Process apps
	var apps []AmplifyAppInfo
	for _, app := range listAppsResp.Apps {
		// Skip if app doesn't have the required tags
		if !hasEnvironmentTag(app.Tags, env) {
			continue
		}

		appInfo := AmplifyAppInfo{
			AppID:          aws.ToString(app.AppId),
			Name:           aws.ToString(app.Name),
			DefaultDomain:  aws.ToString(app.DefaultDomain),
			Repository:     aws.ToString(app.Repository),
			CreateTime:     aws.ToTime(app.CreateTime),
			LastUpdateTime: aws.ToTime(app.UpdateTime),
			Branches:       []AmplifyBranchInfo{},
		}

		// Get custom domain associations
		domainResp, err := amplifyClient.ListDomainAssociations(ctx, &amplify.ListDomainAssociationsInput{
			AppId: app.AppId,
		})
		if err == nil && domainResp.DomainAssociations != nil && len(domainResp.DomainAssociations) > 0 {
			appInfo.CustomDomain = aws.ToString(domainResp.DomainAssociations[0].DomainName)
		}

		// List branches for this app
		listBranchesResp, err := amplifyClient.ListBranches(ctx, &amplify.ListBranchesInput{
			AppId: app.AppId,
		})
		if err != nil {
			// Log error but continue with other apps
			fmt.Printf("Failed to list branches for app %s: %v\n", aws.ToString(app.Name), err)
			continue
		}

		// Process branches
		for _, branch := range listBranchesResp.Branches {
			branchInfo := AmplifyBranchInfo{
				BranchName:               aws.ToString(branch.BranchName),
				Stage:                    string(branch.Stage),
				DisplayName:              aws.ToString(branch.DisplayName),
				EnableAutoBuild:          aws.ToBool(branch.EnableAutoBuild),
				EnablePullRequestPreview: aws.ToBool(branch.EnablePullRequestPreview),
				CreateTime:               aws.ToTime(branch.CreateTime),
				UpdateTime:               aws.ToTime(branch.UpdateTime),
			}

			// Construct branch URL
			branchInfo.BranchURL = fmt.Sprintf("https://%s.%s", branchInfo.BranchName, appInfo.DefaultDomain)

			// Get latest build job for the branch
			listJobsResp, err := amplifyClient.ListJobs(ctx, &amplify.ListJobsInput{
				AppId:      app.AppId,
				BranchName: branch.BranchName,
				MaxResults: int32(1), // Get only the latest job
			})
			if err == nil && len(listJobsResp.JobSummaries) > 0 {
				latestJob := listJobsResp.JobSummaries[0]
				branchInfo.LastBuildStatus = string(latestJob.Status)
				
				// Get detailed job info
				if latestJob.JobId != nil {
					jobResp, err := amplifyClient.GetJob(ctx, &amplify.GetJobInput{
						AppId:      app.AppId,
						BranchName: branch.BranchName,
						JobId:      latestJob.JobId,
					})
					if err == nil && jobResp.Job != nil {
						job := jobResp.Job
						if job.Summary != nil {
							branchInfo.LastBuildTime = job.Summary.StartTime
							if job.Summary.EndTime != nil && job.Summary.StartTime != nil {
								duration := job.Summary.EndTime.Sub(*job.Summary.StartTime)
								branchInfo.LastBuildDuration = int32(duration.Seconds())
							}
							branchInfo.LastCommitId = aws.ToString(job.Summary.CommitId)
							branchInfo.LastCommitMessage = aws.ToString(job.Summary.CommitMessage)
							branchInfo.LastCommitTime = job.Summary.CommitTime
						}
					}
				}
			}

			appInfo.Branches = append(appInfo.Branches, branchInfo)
		}

		apps = append(apps, appInfo)
	}

	// Return response
	response := AmplifyAppsResponse{
		Apps: apps,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

// hasEnvironmentTag checks if the app has the specified environment tag
func hasEnvironmentTag(tags map[string]string, env string) bool {
	if tags == nil {
		return false
	}
	
	// Check for Environment tag
	if envTag, ok := tags["Environment"]; ok && envTag == env {
		return true
	}
	
	// Also check for env tag (lowercase)
	if envTag, ok := tags["env"]; ok && envTag == env {
		return true
	}
	
	return false
}

// createAWSConfig creates an AWS config with optional profile
func createAWSConfig(ctx context.Context, profile string) (aws.Config, error) {
	var optFns []func(*config.LoadOptions) error
	
	if profile != "" {
		optFns = append(optFns, config.WithSharedConfigProfile(profile))
	}
	
	return config.LoadDefaultConfig(ctx, optFns...)
}

// getAmplifyBuildLogs handles GET /api/amplify/build-logs
func getAmplifyBuildLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get parameters
	appId := r.URL.Query().Get("appId")
	branchName := r.URL.Query().Get("branchName")
	jobId := r.URL.Query().Get("jobId")
	
	if appId == "" || branchName == "" || jobId == "" {
		http.Error(w, "appId, branchName, and jobId parameters are required", http.StatusBadRequest)
		return
	}

	// Get profile from query parameter
	profile := r.URL.Query().Get("profile")
	
	// Create AWS config
	ctx := context.Background()
	cfg, err := createAWSConfig(ctx, profile)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create AWS config: %v", err), http.StatusInternalServerError)
		return
	}

	// Create Amplify client
	amplifyClient := amplify.NewFromConfig(cfg)

	// Get job details
	jobResp, err := amplifyClient.GetJob(ctx, &amplify.GetJobInput{
		AppId:      aws.String(appId),
		BranchName: aws.String(branchName),
		JobId:      aws.String(jobId),
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get job details: %v", err), http.StatusInternalServerError)
		return
	}

	// Extract log URL from job steps
	logUrl := ""
	if jobResp.Job != nil && jobResp.Job.Steps != nil {
		for _, step := range jobResp.Job.Steps {
			if step.LogUrl != nil && *step.LogUrl != "" {
				logUrl = *step.LogUrl
				break
			}
		}
	}

	response := map[string]interface{}{
		"logUrl": logUrl,
		"job":    jobResp.Job,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}

// triggerAmplifyBuild handles POST /api/amplify/trigger-build
func triggerAmplifyBuild(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req struct {
		AppId      string `json:"appId"`
		BranchName string `json:"branchName"`
		Profile    string `json:"profile,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.AppId == "" || req.BranchName == "" {
		http.Error(w, "appId and branchName are required", http.StatusBadRequest)
		return
	}

	// Create AWS config
	ctx := context.Background()
	cfg, err := createAWSConfig(ctx, req.Profile)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create AWS config: %v", err), http.StatusInternalServerError)
		return
	}

	// Create Amplify client
	amplifyClient := amplify.NewFromConfig(cfg)

	// Start build
	startJobResp, err := amplifyClient.StartJob(ctx, &amplify.StartJobInput{
		AppId:      aws.String(req.AppId),
		BranchName: aws.String(req.BranchName),
		JobType:    types.JobTypeRelease,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to trigger build: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"jobId":   aws.ToString(startJobResp.JobSummary.JobId),
		"status":  string(startJobResp.JobSummary.Status),
		"message": "Build triggered successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, fmt.Sprintf("Failed to encode response: %v", err), http.StatusInternalServerError)
		return
	}
}