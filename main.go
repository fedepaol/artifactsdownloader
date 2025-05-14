package main

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/go-github/v55/github"
	"golang.org/x/oauth2"
)

func main() {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		fmt.Println("Please set the GITHUB_TOKEN environment variable.")
		os.Exit(1)
	}

	if len(os.Args) < 4 {
		fmt.Println("Usage: go run main.go <repoOwner> <repoName> <runID>")
		os.Exit(1)
	}
	var artifactRegex *regexp.Regexp
	if len(os.Args) >= 5 {
		var err error
		artifactRegex, err = regexp.Compile(os.Args[4])
		if err != nil {
			fmt.Printf("Invalid regex: %v\n", err)
			os.Exit(1)
		}
	} else {
		artifactRegex = regexp.MustCompile(".*") // match all by default
	}

	repoOwner := os.Args[1]
	repoName := os.Args[2]
	runID, err := strconv.ParseInt(os.Args[3], 10, 64)
	if err != nil {
		fmt.Printf("Invalid runID: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	run, _, err := client.Actions.GetWorkflowRunByID(ctx, repoOwner, repoName, runID)
	if err != nil {
		fmt.Printf("Error fetching workflow run: %v\n", err)
		os.Exit(1)
	}

	if run.GetConclusion() != "failure" {
		fmt.Println("Workflow run did not fail.")
		return
	}

	// Fetch logs
	logsURL, _, err := client.Actions.GetWorkflowRunLogs(ctx, repoOwner, repoName, runID, false)
	if err != nil {
		fmt.Printf("Error fetching logs URL: %v\n", err)
		return
	}
	resp, err := http.Get(logsURL.String())
	if err != nil {
		fmt.Printf("Error downloading logs: %v\n", err)
		return
	}
	defer resp.Body.Close()
	logFile, _ := os.Create("workflow_logs.zip")
	io.Copy(logFile, resp.Body)
	logFile.Close()
	fmt.Println("Downloaded workflow logs to workflow_logs.zip")

	// Get list of failed jobs
	jobs, _, err := client.Actions.ListWorkflowJobs(ctx, repoOwner, repoName, runID, nil)
	if err != nil {
		panic(err)
	}
	failedJobs := []string{}
	for _, job := range jobs.Jobs {
		if job.GetConclusion() == "failure" {
			failedJobs = append(failedJobs, job.GetName())
		}
	}
	fmt.Printf("Failed jobs: %v\n", failedJobs)

	// Decompress logs into folders per job
	err = decompressLogsPerJob("workflow_logs.zip", func(jobName string) bool {
		for _, failedName := range failedJobs {
			if strings.Contains(jobName, failedName) {
				return false // Filter out this job
			}
		}
		return true // Include this job
	})

	if err != nil {
		fmt.Printf("Error decompressing logs: %v\n", err)
	}

	// Fetch artifacts
	artifacts, _, err := client.Actions.ListWorkflowRunArtifacts(ctx, repoOwner, repoName, runID, nil)
	if err != nil {
		fmt.Printf("Error listing artifacts: %v\n", err)
		return
	}
	for _, artifact := range artifacts.Artifacts {
		if artifactRegex.MatchString(artifact.GetName()) {
			fmt.Printf("Skipping artifact: %s\n", artifact.GetName())
			continue
		}
		fmt.Printf("Downloading artifact: %s\n", artifact.GetName())
		artifactURL, _, err := client.Actions.DownloadArtifact(ctx, repoOwner, repoName, artifact.GetID(), true)
		if err != nil {
			fmt.Printf("Error downloading artifact: %v\n", err)
			continue
		}
		resp, err := http.Get(artifactURL.String())
		if err != nil {
			fmt.Printf("Error downloading artifact: %v\n", err)
			continue
		}
		defer resp.Body.Close()

		zipReader, err := zip.NewReader(bytes.NewReader(readAll(resp.Body)), resp.ContentLength)
		if err != nil {
			fmt.Printf("Error reading zip: %v\n", err)
			continue
		}
		artifactDir := artifact.GetName()
		os.MkdirAll(artifactDir, 0755)
		for _, f := range zipReader.File {
			fpath := filepath.Join(artifactDir, f.Name)
			if f.FileInfo().IsDir() {
				os.MkdirAll(fpath, f.Mode())
				continue
			}
			if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
				fmt.Printf("Error creating directory: %v\n", err)
				continue
			}
			outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				fmt.Printf("Error creating file: %v\n", err)
				continue
			}
			rc, err := f.Open()
			if err != nil {
				fmt.Printf("Error opening zipped file: %v\n", err)
				outFile.Close()
				continue
			}
			_, err = io.Copy(outFile, rc)
			outFile.Close()
			rc.Close()
			if err != nil {
				fmt.Printf("Error extracting file: %v\n", err)
				continue
			}
		}
	}
}

// decompressLogsPerJob extracts logs into folders per job name
func decompressLogsPerJob(zipPath string, filter func(jobName string) bool) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		parts := strings.SplitN(f.Name, "/", 2)
		if len(parts) < 2 {
			continue
		}
		jobName := parts[0]
		fileName := parts[1]
		if fileName == "" || strings.HasSuffix(fileName, "/") {
			continue
		}
		if filter(jobName) {
			continue
		}
		jobDir := filepath.Join("logs", jobName)
		if err := os.MkdirAll(jobDir, 0755); err != nil {
			fmt.Printf("Error creating job dir: %v\n", err)
			continue
		}
		destPath := filepath.Join(jobDir, fileName)
		outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			fmt.Printf("Error creating log file: %v\n", err)
			continue
		}
		rc, err := f.Open()
		if err != nil {
			fmt.Printf("Error opening zipped log: %v\n", err)
			outFile.Close()
			continue
		}
		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			fmt.Printf("Error extracting log file: %v\n", err)
			continue
		}
	}
	return nil
}

func readAll(r io.Reader) []byte {
	buf := new(bytes.Buffer)
	buf.ReadFrom(r)
	return buf.Bytes()
}
