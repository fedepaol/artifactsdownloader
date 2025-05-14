# GitHub Actions Failed Workflow Log & Artifact Downloader

This program downloads the logs and artifacts of all failed workflow runs for a given GitHub Actions workflow.

## Features

- Takes a workflow ID as input.
- Finds all workflow runs for the given workflow.
- Filters for failed workflow runs.
- Downloads the logs for each failed run.
- Extracts only the logs for failed jobs into a separate folder per job.
- Downloads all artifacts attached to each failed workflow run.

## Usage

1. **Set up your GitHub token**  
   Export your GitHub personal access token as an environment variable:
   ```sh
   export GITHUB_TOKEN=your_token_here
   ```

2. **Run the program**  
   ```sh
   go install github.com/fedepaol/artifactsdownloader@latest

   artifactsdownloader <owner> <repo> <workflow_id>
   ```
   - `<owner>`: GitHub repository owner (e.g., `octocat`)
   - `<repo>`: Repository name (e.g., `hello-world`)
   - `<workflow_id>`: Workflow ID (numeric, see the Actions tab or API)

   The program also accepts an optional regex to be matched against the artifacts to skip:

    ```sh
    artifactsdownloader <owner> <repo> <workflow_id> <filter_regex>
    ```

3. **Output**  
   - Logs for failed jobs are extracted into `logs/<job_name>/`.
   - Artifacts for each failed workflow run are downloaded into `artifacts/<run_id>/`.

## Requirements

- Go 1.18 or newer
- A GitHub personal access token with `repo` and `actions` scopes

## License

APACHE

