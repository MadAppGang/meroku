package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cloudwatchtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/rds"
)

// ---------------------------------------------------------------------------
// Data structs
// ---------------------------------------------------------------------------

// DashboardState holds all data for the current refresh cycle.
type DashboardState struct {
	LoadedAt    time.Time
	Cluster     MonitorCluster
	Services    []MonitorService
	Database    *MonitorDatabase // nil if not configured
	Schedules   []MonitorSchedule
	Deployments []DeploymentEvent
	Errors      []string // non-fatal AWS errors
}

// MonitorCluster summarises the ECS cluster.
type MonitorCluster struct {
	Name           string
	Status         string // "ACTIVE", "INACTIVE"
	RunningTasks   int32
	ActiveServices int32
}

// MonitorService represents one ECS service (backend or named service).
type MonitorService struct {
	Name         string // logical name: "backend", "api", etc.
	ECSName      string // full ECS name e.g. "myapp_service_api_dev"
	Status       string // "ACTIVE", "DRAINING"
	DesiredCount int32
	RunningCount int32
	PendingCount int32
	TaskDef      string
	CPUPercent   float64 // from CloudWatch, -1 if unavailable
	MemPercent   float64 // from CloudWatch, -1 if unavailable
	Tasks        []MonitorTask
}

// MonitorTask represents a running ECS task.
type MonitorTask struct {
	TaskID        string // short ID (last 12 chars of ARN)
	TaskArn       string // full ARN (needed for ECS Exec)
	ContainerName string
	Status        string
	StartedAt     *time.Time
	AZ            string
	PrivateIP     string
	CPU           string
	Memory        string
}

// MonitorDatabase holds RDS/Aurora status.
type MonitorDatabase struct {
	Identifier    string
	Status        string
	Engine        string
	EngineVersion string
	Endpoint      string
	Port          int32
	IsAurora      bool
	// CloudWatch metrics (nil if unavailable)
	CPUPercent       *float64
	Connections      *float64
	FreeStorageBytes *float64
}

// MonitorSchedule holds EventBridge scheduled rule info.
type MonitorSchedule struct {
	Name       string
	Schedule   string
	State      string // "ENABLED", "DISABLED"
	GroupName  string
	LastStatus string
	NextRunAt  *time.Time
}

// DeploymentEvent represents a deployment step/event.
type DeploymentEvent struct {
	Timestamp   time.Time
	Source      string // "ECS", "GitHub", "ECR"
	EventType   string
	ServiceName string
	Message     string
	Status      string // "running", "success", "failure", "pending"
}

// WorkflowRun represents a GitHub Actions workflow run.
type WorkflowRun struct {
	ID          int64      `json:"databaseId"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	StartedAt   *time.Time `json:"startedAt"`
	CompletedAt *time.Time `json:"updatedAt"`
	HTMLURL     string     `json:"url"`
	HeadSHA     string     `json:"headSha"`
	HeadBranch  string     `json:"headBranch"`
}

// LogPage holds a page of CloudWatch logs.
type LogPage struct {
	ServiceName string
	Entries     []MonitorLogEntry
	NextToken   string
	Error       error
}

// MonitorLogEntry is a single log line.
type MonitorLogEntry struct {
	Timestamp time.Time
	Message   string
	Level     string // "ERROR", "WARN", "INFO", "DEBUG", ""
	Stream    string
}

// monitorServiceMetrics holds CloudWatch metrics for a service (monitor-specific, avoids clash with api_autoscaling.go).
type monitorServiceMetrics struct {
	CPUPercent  float64
	MemPercent  float64
	CollectedAt time.Time
}

// ---------------------------------------------------------------------------
// AWS config helper
// ---------------------------------------------------------------------------

// buildAWSConfig creates an AWS SDK config using the given profile and region.
func buildAWSConfig(ctx context.Context, profile, region string) (aws.Config, error) {
	opts := []func(*config.LoadOptions) error{}
	if profile != "" {
		opts = append(opts, config.WithSharedConfigProfile(profile))
	}
	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}
	return config.LoadDefaultConfig(ctx, opts...)
}

// ---------------------------------------------------------------------------
// Main orchestrator
// ---------------------------------------------------------------------------

// fetchDashboardData fetches all data for the Overview and Deployment views.
// It runs sub-fetches concurrently where possible.
func fetchDashboardData(ctx context.Context, env Env, profile string) (DashboardState, error) {
	state := DashboardState{
		LoadedAt: time.Now(),
	}

	cfg, err := buildAWSConfig(ctx, profile, env.Region)
	if err != nil {
		state.Errors = append(state.Errors, fmt.Sprintf("AWS config: %v", err))
		return state, nil
	}

	ecsClient := ecs.NewFromConfig(cfg)
	rdsClient := rds.NewFromConfig(cfg)
	cwClient := cloudwatch.NewFromConfig(cfg)
	ebClient := eventbridge.NewFromConfig(cfg)

	var mu sync.Mutex
	var wg sync.WaitGroup

	// Fetch ECS services
	wg.Add(1)
	go func() {
		defer wg.Done()
		services, err := fetchECSServices(ctx, ecsClient, env)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			state.Errors = append(state.Errors, fmt.Sprintf("ECS services: %v", err))
		} else {
			state.Services = services
		}
	}()

	// Fetch cluster info (embedded in DescribeClusters)
	wg.Add(1)
	go func() {
		defer wg.Done()
		cluster, err := fetchECSCluster(ctx, ecsClient, env)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			state.Errors = append(state.Errors, fmt.Sprintf("ECS cluster: %v", err))
		} else {
			state.Cluster = cluster
		}
	}()

	// Fetch RDS status (only if postgres is enabled)
	if env.Postgres.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db, err := fetchRDSStatus(ctx, rdsClient, env)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				state.Errors = append(state.Errors, fmt.Sprintf("RDS: %v", err))
			} else {
				state.Database = db
			}
		}()
	}

	// Fetch scheduled tasks via EventBridge rules
	if len(env.ScheduledTasks) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			schedules, err := fetchScheduledTasks(ctx, ebClient, env)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				state.Errors = append(state.Errors, fmt.Sprintf("Schedules: %v", err))
			} else {
				state.Schedules = schedules
			}
		}()
	}

	// Fetch deployment events
	wg.Add(1)
	go func() {
		defer wg.Done()
		events, err := fetchECSDeploymentEvents(ctx, ecsClient, env)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			state.Errors = append(state.Errors, fmt.Sprintf("Deployment events: %v", err))
		} else {
			state.Deployments = events
		}
	}()

	wg.Wait()

	// After services fetched, fetch tasks and metrics concurrently (per service)
	if len(state.Services) > 0 {
		var svcWg sync.WaitGroup
		semaphore := make(chan struct{}, 3) // max 3 concurrent metric fetches

		clusterName := fmt.Sprintf("%s_cluster_%s", env.Project, env.Env)

		for i := range state.Services {
			svcWg.Add(1)
			go func(idx int) {
				defer svcWg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				svc := &state.Services[idx]

				// Fetch tasks
				tasks, err := fetchECSTasksForService(ctx, ecsClient, clusterName, svc.ECSName)
				mu.Lock()
				if err != nil {
					state.Errors = append(state.Errors, fmt.Sprintf("Tasks for %s: %v", svc.Name, err))
				} else {
					svc.Tasks = tasks
				}
				mu.Unlock()

				// Fetch CloudWatch metrics
				metrics, err := fetchServiceMetrics(ctx, cwClient, clusterName, svc.ECSName)
				mu.Lock()
				if err == nil {
					svc.CPUPercent = metrics.CPUPercent
					svc.MemPercent = metrics.MemPercent
				} else {
					svc.CPUPercent = -1
					svc.MemPercent = -1
				}
				mu.Unlock()
			}(i)
		}
		svcWg.Wait()
	}

	return state, nil
}

// ---------------------------------------------------------------------------
// ECS Cluster
// ---------------------------------------------------------------------------

func fetchECSCluster(ctx context.Context, client *ecs.Client, env Env) (MonitorCluster, error) {
	clusterName := fmt.Sprintf("%s_cluster_%s", env.Project, env.Env)
	out, err := client.DescribeClusters(ctx, &ecs.DescribeClustersInput{
		Clusters: []string{clusterName},
	})
	if err != nil {
		return MonitorCluster{Name: clusterName}, err
	}
	if len(out.Clusters) == 0 {
		return MonitorCluster{Name: clusterName, Status: "NOT_FOUND"}, nil
	}
	c := out.Clusters[0]
	return MonitorCluster{
		Name:           aws.ToString(c.ClusterName),
		Status:         aws.ToString(c.Status),
		RunningTasks:   c.RunningTasksCount,
		ActiveServices: c.ActiveServicesCount,
	}, nil
}

// ---------------------------------------------------------------------------
// ECS Services
// ---------------------------------------------------------------------------

// fetchECSServices returns all ECS services in the cluster.
func fetchECSServices(ctx context.Context, client *ecs.Client, env Env) ([]MonitorService, error) {
	clusterName := fmt.Sprintf("%s_cluster_%s", env.Project, env.Env)

	// List all service ARNs
	var serviceArns []string
	paginator := ecs.NewListServicesPaginator(client, &ecs.ListServicesInput{
		Cluster: aws.String(clusterName),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list services: %w", err)
		}
		serviceArns = append(serviceArns, page.ServiceArns...)
	}

	if len(serviceArns) == 0 {
		return []MonitorService{}, nil
	}

	// DescribeServices (max 10 per call)
	var services []MonitorService
	for i := 0; i < len(serviceArns); i += 10 {
		end := i + 10
		if end > len(serviceArns) {
			end = len(serviceArns)
		}
		out, err := client.DescribeServices(ctx, &ecs.DescribeServicesInput{
			Cluster:  aws.String(clusterName),
			Services: serviceArns[i:end],
		})
		if err != nil {
			return nil, fmt.Errorf("describe services: %w", err)
		}
		for _, svc := range out.Services {
			ecsName := aws.ToString(svc.ServiceName)
			logicalName := ecsServiceLogicalName(ecsName, env)
			taskDef := aws.ToString(svc.TaskDefinition)
			// Shorten task def ARN to last segment
			if parts := strings.Split(taskDef, "/"); len(parts) > 0 {
				taskDef = parts[len(parts)-1]
			}
			services = append(services, MonitorService{
				Name:         logicalName,
				ECSName:      ecsName,
				Status:       aws.ToString(svc.Status),
				DesiredCount: svc.DesiredCount,
				RunningCount: svc.RunningCount,
				PendingCount: svc.PendingCount,
				TaskDef:      taskDef,
				CPUPercent:   -1,
				MemPercent:   -1,
			})
		}
	}
	return services, nil
}

// ecsServiceLogicalName converts an ECS service name to a logical short name.
// e.g. "myapp_service_api_dev" -> "api", "myapp_service_dev" -> "backend"
func ecsServiceLogicalName(ecsName string, env Env) string {
	backendName := fmt.Sprintf("%s_service_%s", env.Project, env.Env)
	if ecsName == backendName {
		return "backend"
	}
	prefix := fmt.Sprintf("%s_service_", env.Project)
	suffix := fmt.Sprintf("_%s", env.Env)
	if strings.HasPrefix(ecsName, prefix) && strings.HasSuffix(ecsName, suffix) {
		middle := ecsName[len(prefix) : len(ecsName)-len(suffix)]
		return middle
	}
	return ecsName
}

// ---------------------------------------------------------------------------
// ECS Tasks
// ---------------------------------------------------------------------------

// fetchECSTasksForService returns running tasks for one ECS service.
func fetchECSTasksForService(ctx context.Context, client *ecs.Client, cluster, serviceArn string) ([]MonitorTask, error) {
	listOut, err := client.ListTasks(ctx, &ecs.ListTasksInput{
		Cluster:     aws.String(cluster),
		ServiceName: aws.String(serviceArn),
	})
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	if len(listOut.TaskArns) == 0 {
		return []MonitorTask{}, nil
	}

	descOut, err := client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(cluster),
		Tasks:   listOut.TaskArns,
	})
	if err != nil {
		return nil, fmt.Errorf("describe tasks: %w", err)
	}

	var tasks []MonitorTask
	for _, t := range descOut.Tasks {
		taskArn := aws.ToString(t.TaskArn)
		taskID := taskArn
		// Take last 12 chars of the task ID
		if parts := strings.Split(taskArn, "/"); len(parts) > 0 {
			id := parts[len(parts)-1]
			if len(id) > 12 {
				taskID = id[:12]
			} else {
				taskID = id
			}
		}

		containerName := ""
		if len(t.Containers) > 0 {
			containerName = aws.ToString(t.Containers[0].Name)
		}

		privateIP := ""
		if len(t.Attachments) > 0 {
			for _, att := range t.Attachments {
				for _, detail := range att.Details {
					if aws.ToString(detail.Name) == "privateIPv4Address" {
						privateIP = aws.ToString(detail.Value)
					}
				}
			}
		}

		tasks = append(tasks, MonitorTask{
			TaskID:        taskID,
			TaskArn:       taskArn,
			ContainerName: containerName,
			Status:        aws.ToString(t.LastStatus),
			StartedAt:     t.StartedAt,
			AZ:            aws.ToString(t.AvailabilityZone),
			PrivateIP:     privateIP,
			CPU:           aws.ToString(t.Cpu),
			Memory:        aws.ToString(t.Memory),
		})
	}
	return tasks, nil
}

// ---------------------------------------------------------------------------
// RDS
// ---------------------------------------------------------------------------

// fetchRDSStatus returns database status or nil if postgres is not enabled.
func fetchRDSStatus(ctx context.Context, client *rds.Client, env Env) (*MonitorDatabase, error) {
	if !env.Postgres.Enabled {
		return nil, nil
	}

	if env.Postgres.Aurora {
		return fetchAuroraStatus(ctx, client, env)
	}
	return fetchRDSInstanceStatus(ctx, client, env)
}

func fetchAuroraStatus(ctx context.Context, client *rds.Client, env Env) (*MonitorDatabase, error) {
	identifier := fmt.Sprintf("%s-aurora-%s", env.Project, env.Env)
	out, err := client.DescribeDBClusters(ctx, &rds.DescribeDBClustersInput{
		DBClusterIdentifier: aws.String(identifier),
	})
	if err != nil {
		// Try alternate naming
		identifier = fmt.Sprintf("%s-%s-cluster", env.Project, env.Env)
		out, err = client.DescribeDBClusters(ctx, &rds.DescribeDBClustersInput{
			DBClusterIdentifier: aws.String(identifier),
		})
		if err != nil {
			return nil, fmt.Errorf("aurora cluster not found: %w", err)
		}
	}
	if len(out.DBClusters) == 0 {
		return nil, nil
	}
	c := out.DBClusters[0]
	endpoint := ""
	if c.Endpoint != nil {
		endpoint = aws.ToString(c.Endpoint)
	}
	return &MonitorDatabase{
		Identifier:    aws.ToString(c.DBClusterIdentifier),
		Status:        aws.ToString(c.Status),
		Engine:        aws.ToString(c.Engine),
		EngineVersion: aws.ToString(c.EngineVersion),
		Endpoint:      endpoint,
		Port:          aws.ToInt32(c.Port),
		IsAurora:      true,
	}, nil
}

func fetchRDSInstanceStatus(ctx context.Context, client *rds.Client, env Env) (*MonitorDatabase, error) {
	identifier := fmt.Sprintf("%s-postgres-%s", env.Project, env.Env)
	out, err := client.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: aws.String(identifier),
	})
	if err != nil {
		identifier = fmt.Sprintf("%s-%s-rds", env.Project, env.Env)
		out, err = client.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{
			DBInstanceIdentifier: aws.String(identifier),
		})
		if err != nil {
			return nil, fmt.Errorf("rds instance not found: %w", err)
		}
	}
	if len(out.DBInstances) == 0 {
		return nil, nil
	}
	inst := out.DBInstances[0]
	endpoint := ""
	if inst.Endpoint != nil {
		endpoint = aws.ToString(inst.Endpoint.Address)
	}
	port := int32(5432)
	if inst.Endpoint != nil {
		port = aws.ToInt32(inst.Endpoint.Port)
	}
	return &MonitorDatabase{
		Identifier:    aws.ToString(inst.DBInstanceIdentifier),
		Status:        aws.ToString(inst.DBInstanceStatus),
		Engine:        aws.ToString(inst.Engine),
		EngineVersion: aws.ToString(inst.EngineVersion),
		Endpoint:      endpoint,
		Port:          port,
		IsAurora:      false,
	}, nil
}

// ---------------------------------------------------------------------------
// Scheduled Tasks (EventBridge Rules)
// ---------------------------------------------------------------------------

// fetchScheduledTasks returns EventBridge scheduled rule state using ListRules.
func fetchScheduledTasks(ctx context.Context, client *eventbridge.Client, env Env) ([]MonitorSchedule, error) {
	var schedules []MonitorSchedule

	for _, task := range env.ScheduledTasks {
		// EventBridge rule names follow: project-schedule-group-env-name pattern for groups
		// but the rule itself can vary. Try common patterns.
		ruleName := fmt.Sprintf("%s-%s-%s-schedule", env.Project, env.Env, task.Name)

		out, err := client.ListRules(ctx, &eventbridge.ListRulesInput{
			NamePrefix: aws.String(ruleName),
		})

		var schedule MonitorSchedule
		if err != nil || len(out.Rules) == 0 {
			// Try alternate pattern
			altName := fmt.Sprintf("%s_%s_%s", env.Project, task.Name, env.Env)
			out2, err2 := client.ListRules(ctx, &eventbridge.ListRulesInput{
				NamePrefix: aws.String(altName),
			})
			if err2 != nil || len(out2.Rules) == 0 {
				// Add placeholder entry showing the task exists
				schedule = MonitorSchedule{
					Name:      task.Name,
					Schedule:  task.Schedule,
					State:     "UNKNOWN",
					GroupName: fmt.Sprintf("%s-schedule-group-%s-%s", env.Project, env.Env, task.Name),
				}
			} else {
				rule := out2.Rules[0]
				schedule = MonitorSchedule{
					Name:      task.Name,
					Schedule:  aws.ToString(rule.ScheduleExpression),
					State:     string(rule.State),
					GroupName: fmt.Sprintf("%s-schedule-group-%s-%s", env.Project, env.Env, task.Name),
				}
			}
		} else {
			rule := out.Rules[0]
			schedule = MonitorSchedule{
				Name:      task.Name,
				Schedule:  aws.ToString(rule.ScheduleExpression),
				State:     string(rule.State),
				GroupName: fmt.Sprintf("%s-schedule-group-%s-%s", env.Project, env.Env, task.Name),
			}
		}

		// Use configured schedule if we couldn't get it from AWS
		if schedule.Schedule == "" {
			schedule.Schedule = task.Schedule
		}

		schedules = append(schedules, schedule)
	}

	return schedules, nil
}

// ---------------------------------------------------------------------------
// CloudWatch Metrics
// ---------------------------------------------------------------------------

// fetchServiceMetrics returns CPU and memory utilization for one ECS service.
func fetchServiceMetrics(ctx context.Context, cwClient *cloudwatch.Client, cluster, service string) (monitorServiceMetrics, error) {
	now := time.Now()
	start := now.Add(-10 * time.Minute)
	period := int32(300)

	metrics := []struct {
		metricName string
		stat       string
	}{
		{"CPUUtilization", "Average"},
		{"MemoryUtilization", "Average"},
	}

	result := monitorServiceMetrics{CollectedAt: now}

	for i, m := range metrics {
		out, err := cwClient.GetMetricStatistics(ctx, &cloudwatch.GetMetricStatisticsInput{
			Namespace:  aws.String("AWS/ECS"),
			MetricName: aws.String(m.metricName),
			Dimensions: []cloudwatchtypes.Dimension{
				{Name: aws.String("ClusterName"), Value: aws.String(cluster)},
				{Name: aws.String("ServiceName"), Value: aws.String(service)},
			},
			StartTime:  aws.Time(start),
			EndTime:    aws.Time(now),
			Period:     aws.Int32(period),
			Statistics: []cloudwatchtypes.Statistic{cloudwatchtypes.StatisticAverage},
		})
		if err != nil || len(out.Datapoints) == 0 {
			continue
		}
		// Use the most recent datapoint
		var latest *cloudwatchtypes.Datapoint
		for k := range out.Datapoints {
			dp := &out.Datapoints[k]
			if latest == nil || dp.Timestamp.After(*latest.Timestamp) {
				latest = dp
			}
		}
		if latest != nil && latest.Average != nil {
			if i == 0 {
				result.CPUPercent = *latest.Average
			} else {
				result.MemPercent = *latest.Average
			}
		}
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// CloudWatch Logs
// ---------------------------------------------------------------------------

// fetchLogsPage retrieves one page of CloudWatch Logs for a service.
func fetchLogsPage(ctx context.Context, cwlClient *cloudwatchlogs.Client, env Env, serviceName, nextToken string, limit int32) (LogPage, error) {
	logGroupName := constructLogGroupName(env, serviceName)

	if limit <= 0 {
		limit = 100
	}

	input := &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName: aws.String(logGroupName),
		Limit:        aws.Int32(limit),
	}
	if nextToken != "" {
		input.NextToken = aws.String(nextToken)
	}

	out, err := cwlClient.FilterLogEvents(ctx, input)
	if err != nil {
		return LogPage{ServiceName: serviceName}, err
	}

	var entries []MonitorLogEntry
	for _, evt := range out.Events {
		if evt.Message == nil || evt.Timestamp == nil {
			continue
		}
		msg := strings.TrimSpace(aws.ToString(evt.Message))
		ts := time.Unix(*evt.Timestamp/1000, 0)
		entries = append(entries, MonitorLogEntry{
			Timestamp: ts,
			Message:   msg,
			Level:     detectLogLevel(msg),
			Stream:    aws.ToString(evt.LogStreamName),
		})
	}

	outToken := ""
	if out.NextToken != nil {
		outToken = *out.NextToken
	}

	return LogPage{
		ServiceName: serviceName,
		Entries:     entries,
		NextToken:   outToken,
	}, nil
}

// detectLogLevel infers log level from message content.
func detectLogLevel(msg string) string {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "error") || strings.Contains(lower, "exception") || strings.Contains(lower, "fatal") || strings.Contains(lower, "panic"):
		return "ERROR"
	case strings.Contains(lower, "warn"):
		return "WARN"
	case strings.Contains(lower, "debug"):
		return "DEBUG"
	default:
		return "INFO"
	}
}

// ---------------------------------------------------------------------------
// ECS Deployment Events
// ---------------------------------------------------------------------------

// fetchECSDeploymentEvents returns recent ECS service events.
func fetchECSDeploymentEvents(ctx context.Context, ecsClient *ecs.Client, env Env) ([]DeploymentEvent, error) {
	clusterName := fmt.Sprintf("%s_cluster_%s", env.Project, env.Env)

	// List up to 10 services (sufficient for event display)
	listOut, err := ecsClient.ListServices(ctx, &ecs.ListServicesInput{
		Cluster:    aws.String(clusterName),
		MaxResults: aws.Int32(10),
	})
	if err != nil {
		return nil, fmt.Errorf("list services for events: %w", err)
	}
	if len(listOut.ServiceArns) == 0 {
		return []DeploymentEvent{}, nil
	}

	arns := listOut.ServiceArns

	descOut, err := ecsClient.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster:  aws.String(clusterName),
		Services: arns,
	})
	if err != nil {
		return nil, fmt.Errorf("describe services for events: %w", err)
	}

	var events []DeploymentEvent
	for _, svc := range descOut.Services {
		svcName := ecsServiceLogicalName(aws.ToString(svc.ServiceName), env)
		// Take up to 5 events per service
		limit := 5
		if len(svc.Events) < limit {
			limit = len(svc.Events)
		}
		for _, evt := range svc.Events[:limit] {
			status := "success"
			msg := aws.ToString(evt.Message)
			lmsg := strings.ToLower(msg)
			if strings.Contains(lmsg, "failed") || strings.Contains(lmsg, "error") || strings.Contains(lmsg, "unable") {
				status = "failure"
			} else if strings.Contains(lmsg, "started") || strings.Contains(lmsg, "deploying") {
				status = "running"
			}
			ts := time.Now()
			if evt.CreatedAt != nil {
				ts = *evt.CreatedAt
			}
			events = append(events, DeploymentEvent{
				Timestamp:   ts,
				Source:      "ECS",
				EventType:   "SERVICE_EVENT",
				ServiceName: svcName,
				Message:     msg,
				Status:      status,
			})
		}
	}

	// Sort by timestamp descending
	for i := 0; i < len(events); i++ {
		for j := i + 1; j < len(events); j++ {
			if events[j].Timestamp.After(events[i].Timestamp) {
				events[i], events[j] = events[j], events[i]
			}
		}
	}

	return events, nil
}

// ---------------------------------------------------------------------------
// GitHub Workflows
// ---------------------------------------------------------------------------

// fetchGitHubWorkflows runs `gh run list --json ...` via os/exec and parses output.
// Returns empty slice (not error) if gh CLI is not installed or repo not configured.
func fetchGitHubWorkflows(env Env) ([]WorkflowRun, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, nil
	}

	cmd := exec.Command("gh", "run", "list",
		"--limit", "10",
		"--json", "databaseId,status,conclusion,name,startedAt,updatedAt,url,headSha,headBranch",
	)
	output, err := cmd.Output()
	if err != nil {
		return nil, nil
	}

	var runs []WorkflowRun
	if err := json.Unmarshal(output, &runs); err != nil {
		return nil, nil
	}
	return runs, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// formatUptime returns a human-readable uptime string.
func formatUptime(since *time.Time) string {
	if since == nil {
		return "unknown"
	}
	d := time.Since(*since)
	if d < 0 {
		d = 0
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, mins)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, mins)
	}
	return fmt.Sprintf("%dm", mins)
}
