package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetServiceNameFromRepoName(t *testing.T) {
	const project = "myapp"

	tests := []struct {
		name     string
		repoName string
		wantID   string
		wantErr  bool
	}{
		// Backend service
		{
			name:     "backend repo returns empty string",
			repoName: "myapp_backend",
			wantID:   "",
			wantErr:  false,
		},

		// Named ECS services
		{
			name:     "named service returns service name",
			repoName: "myapp_service_api",
			wantID:   "api",
			wantErr:  false,
		},
		{
			name:     "multi-word named service",
			repoName: "myapp_service_payment_worker",
			wantID:   "payment_worker",
			wantErr:  false,
		},

		// Scheduled tasks
		{
			name:     "task repo returns task prefixed identifier",
			repoName: "myapp_task_cleanup",
			wantID:   "task:cleanup",
			wantErr:  false,
		},
		{
			name:     "multi-word task repo",
			repoName: "myapp_task_nightly_report",
			wantID:   "task:nightly_report",
			wantErr:  false,
		},

		// Error cases
		{
			name:     "unknown repo pattern returns error",
			repoName: "myapp_unknown_thing",
			wantID:   "",
			wantErr:  true,
		},
		{
			name:     "unrelated repo name returns error",
			repoName: "other_project_backend",
			wantID:   "",
			wantErr:  true,
		},
		{
			name:     "empty string returns error",
			repoName: "",
			wantID:   "",
			wantErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := GetServiceNameFromRepoName(tc.repoName, project)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantID, got)
		})
	}
}
