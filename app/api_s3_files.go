package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"gopkg.in/yaml.v2"
)

type S3FileInfo struct {
	Bucket       string `json:"bucket"`
	Key          string `json:"key"`
	Size         int64  `json:"size"`
	LastModified string `json:"lastModified"`
	ETag         string `json:"etag"`
}

type S3FileContent struct {
	Bucket  string `json:"bucket"`
	Key     string `json:"key"`
	Content string `json:"content"`
}

type S3FileRequest struct {
	Bucket  string `json:"bucket"`
	Key     string `json:"key"`
	Content string `json:"content"`
}

type S3ListResponse struct {
	Files   []S3FileInfo `json:"files"`
	Folders []string     `json:"folders"`
}

// GET /api/s3/file?bucket=<bucket>&key=<key>
func getS3File(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	bucket := r.URL.Query().Get("bucket")
	key := r.URL.Query().Get("key")

	if bucket == "" || key == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "bucket and key parameters are required"})
		return
	}

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(selectedAWSProfile),
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to load AWS config"})
		return
	}

	s3Client := s3.NewFromConfig(cfg)

	// Get the object using the full bucket name directly
	result, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if strings.Contains(err.Error(), "NoSuchKey") {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "file not found"})
		} else if strings.Contains(err.Error(), "NoSuchBucket") {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "bucket not found"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		}
		return
	}
	defer result.Body.Close()

	// Read the content
	body, err := io.ReadAll(result.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to read file content"})
		return
	}

	response := S3FileContent{
		Bucket:  bucket,
		Key:     key,
		Content: string(body),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// PUT /api/s3/file
func putS3File(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req S3FileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "invalid request body"})
		return
	}

	if req.Bucket == "" || req.Key == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "bucket and key are required"})
		return
	}

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(selectedAWSProfile),
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to load AWS config"})
		return
	}

	s3Client := s3.NewFromConfig(cfg)

	// Put the object using the full bucket name directly
	_, err = s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(req.Bucket),
		Key:    aws.String(req.Key),
		Body:   strings.NewReader(req.Content),
	})
	if err != nil {
		if strings.Contains(err.Error(), "NoSuchBucket") {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "bucket not found"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "file uploaded successfully",
		"bucket":  req.Bucket,
		"key":     req.Key,
	})
}

// DELETE /api/s3/file?bucket=<bucket>&key=<key>
func deleteS3File(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	bucket := r.URL.Query().Get("bucket")
	key := r.URL.Query().Get("key")

	if bucket == "" || key == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "bucket and key parameters are required"})
		return
	}

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(selectedAWSProfile),
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to load AWS config"})
		return
	}

	s3Client := s3.NewFromConfig(cfg)

	// Delete the object using the full bucket name directly
	_, err = s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "file deleted successfully",
		"bucket":  bucket,
		"key":     key,
	})
}

// GET /api/s3/files?bucket=<bucket>&prefix=<prefix>
func listS3Files(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	bucket := r.URL.Query().Get("bucket")
	prefix := r.URL.Query().Get("prefix")

	if bucket == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "bucket parameter is required"})
		return
	}

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(selectedAWSProfile),
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to load AWS config"})
		return
	}

	s3Client := s3.NewFromConfig(cfg)

	// List objects using the full bucket name directly
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	}
	if prefix != "" {
		input.Prefix = aws.String(prefix)
		input.Delimiter = aws.String("/")
	}

	result, err := s3Client.ListObjectsV2(ctx, input)
	if err != nil {
		if strings.Contains(err.Error(), "NoSuchBucket") {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(ErrorResponse{Error: "bucket not found"})
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		}
		return
	}

	response := S3ListResponse{
		Files:   []S3FileInfo{},
		Folders: []string{},
	}

	// Add files
	for _, obj := range result.Contents {
		if obj.Key != nil && !strings.HasSuffix(*obj.Key, "/") {
			fileInfo := S3FileInfo{
				Bucket: bucket,
				Key:    *obj.Key,
				Size:   *obj.Size,
			}
			if obj.LastModified != nil {
				fileInfo.LastModified = obj.LastModified.Format("2006-01-02T15:04:05Z")
			}
			if obj.ETag != nil {
				fileInfo.ETag = *obj.ETag
			}
			response.Files = append(response.Files, fileInfo)
		}
	}

	// Add folders (common prefixes)
	for _, prefix := range result.CommonPrefixes {
		if prefix.Prefix != nil {
			response.Folders = append(response.Folders, *prefix.Prefix)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GET /api/s3/buckets?env=<env>
func listProjectS3Buckets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	envName := r.URL.Query().Get("env")
	if envName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "env parameter is required"})
		return
	}

	// Load environment config to get project name
	filename := fmt.Sprintf("%s.yaml", envName)
	content, err := os.ReadFile(filename)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "environment not found"})
		return
	}

	var envConfig Env
	if err := yaml.Unmarshal(content, &envConfig); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to parse environment config"})
		return
	}

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(selectedAWSProfile),
	)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: "failed to load AWS config"})
		return
	}

	s3Client := s3.NewFromConfig(cfg)

	// List all buckets
	result, err := s3Client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
		return
	}

	// Filter buckets that match the project pattern
	var projectBuckets []string
	prefix := fmt.Sprintf("%s-", envConfig.Project)
	suffix := fmt.Sprintf("-%s", envConfig.Env)

	for _, bucket := range result.Buckets {
		if bucket.Name != nil && strings.HasPrefix(*bucket.Name, prefix) && strings.HasSuffix(*bucket.Name, suffix) {
			projectBuckets = append(projectBuckets, *bucket.Name)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(projectBuckets)
}