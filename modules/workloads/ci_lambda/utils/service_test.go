package utils_test

import (
	"testing"

	"madappgang.com/infrastructure/ci_lambda/utils"
)

func TestServiceNameExtract(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		projectName string
		want        string
		wantErr     bool
	}{
		{
			name:        "backend service",
			input:       "moreai_backend",
			projectName: "moreai",
			want:        "",
			wantErr:     false,
		},
		{
			name:        "regular service",
			input:       "moreai_service_aichat",
			projectName: "moreai",
			want:        "aichat",
			wantErr:     false,
		},
		{
			name:        "task",
			input:       "moreai_task_fees",
			projectName: "moreai",
			want:        "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := utils.GetServiceNameFromRepoName(tt.input, tt.projectName)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetServiceNameFromRepoName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetServiceNameFromRepoName() = %v, want %v", got, tt.want)
			}
		})
	}
}
