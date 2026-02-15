package main

import (
	"encoding/json"
	"net/http"
)

// FargateCPUOption represents a valid Fargate CPU tier with its allowed memory values
type FargateCPUOption struct {
	CPU           int    `json:"cpu"`
	VCPU          string `json:"vcpu"`
	MemoryOptions []int  `json:"memoryOptions"`
}

// FargateOptionsResponse contains all valid Fargate CPU/memory combinations
type FargateOptionsResponse struct {
	Options []FargateCPUOption `json:"options"`
}

// Valid Fargate task size combinations per AWS documentation
// https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task_definition_parameters.html#task_size
var fargateOptions = FargateOptionsResponse{
	Options: []FargateCPUOption{
		{CPU: 256, VCPU: "0.25", MemoryOptions: []int{512, 1024, 2048}},
		{CPU: 512, VCPU: "0.5", MemoryOptions: []int{1024, 2048, 3072, 4096}},
		{CPU: 1024, VCPU: "1", MemoryOptions: []int{2048, 3072, 4096, 5120, 6144, 7168, 8192}},
		{CPU: 2048, VCPU: "2", MemoryOptions: []int{4096, 5120, 6144, 7168, 8192, 9216, 10240, 11264, 12288, 13312, 14336, 15360, 16384}},
		{CPU: 4096, VCPU: "4", MemoryOptions: []int{8192, 9216, 10240, 11264, 12288, 13312, 14336, 15360, 16384, 17408, 18432, 19456, 20480, 21504, 22528, 23552, 24576, 25600, 26624, 27648, 28672, 29696, 30720}},
		{CPU: 8192, VCPU: "8", MemoryOptions: []int{16384, 20480, 24576, 28672, 32768, 36864, 40960, 45056, 49152, 53248, 57344, 61440}},
		{CPU: 16384, VCPU: "16", MemoryOptions: []int{32768, 40960, 49152, 57344, 65536, 73728, 81920, 90112, 98304, 106496, 114688, 122880}},
	},
}

func getFargateOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(fargateOptions)
}
