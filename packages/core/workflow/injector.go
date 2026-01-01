package workflow

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/handleui/detent/packages/core/ci"
	"github.com/goccy/go-yaml"
	"golang.org/x/sync/errgroup"
)

// validJobIDPattern matches GitHub Actions job ID requirements: [a-zA-Z_][a-zA-Z0-9_-]*
// This prevents shell injection via malicious job IDs in marker echo commands.
var validJobIDPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_-]*$`)

// InjectContinueOnError modifies a workflow to add continue-on-error: true to all jobs and steps.
// This ensures that Docker failures, job-level failures, and step-level failures don't stop execution,
// allowing Detent to capture ALL errors instead of just the first failure.
func InjectContinueOnError(wf *Workflow) {
	if wf == nil || wf.Jobs == nil {
		return
	}
	for _, job := range wf.Jobs {
		if job == nil {
			continue
		}

		// Inject at JOB level ONLY (critical for Docker failures and continuing past step failures)
		// Job.ContinueOnError is `any` type to support bool or expressions
		// NOTE: We intentionally do NOT inject at step level because it suppresses step output in act,
		// preventing error extraction. Job-level continue-on-error is sufficient to prevent workflow truncation.
		if job.ContinueOnError == nil || job.ContinueOnError == false {
			job.ContinueOnError = true
		}
	}
}

// buildStringSet creates a set (map[string]struct{}) from a slice for O(1) lookups.
func buildStringSet(items []string) map[string]struct{} {
	m := make(map[string]struct{}, len(items))
	for _, item := range items {
		m[item] = struct{}{}
	}
	return m
}

// sensitiveJobNames contains keywords that indicate a job may publish, release, or deploy.
// Jobs containing these keywords should NOT get if: always() to prevent accidental production releases.
// This list is intentionally comprehensive to err on the side of safety.
var sensitiveJobNames = []string{
	// Core deployment/release terms
	"release", "publish", "deploy", "production", "prod",
	"staging", "ship", "distribute", "upload",
	// Additional deployment contexts
	"live", "canary", "rollout", "blue-green", "bluegreen",
	"promote", "delivery", "push-to", "push_to",
	// Infrastructure and migration terms
	"infra", "migration", "migrate", "scale", "provision",
	// Platform-specific terms
	"npm-publish", "docker-push", "pypi", "rubygems", "nuget",
	"homebrew", "brew-", "cargo-publish", "maven-deploy",
}

// sensitiveActions contains GitHub Actions that perform publishing or deployment.
// Jobs using these actions should NOT get if: always() to prevent accidental production releases.
// This list is intentionally comprehensive to err on the side of safety.
var sensitiveActions = []string{
	// === Package Publishing ===
	// JavaScript/Node.js
	"changesets/action",   // npm releases with changesets
	"JS-DevTools/npm-publish", // npm publishing
	"primer/publish",      // npm publishing (Primer)
	// Go
	"goreleaser/goreleaser-action", // Go releases
	// Python
	"pypa/gh-action-pypi-publish", // PyPI publishing
	// Ruby
	"rubygems/release-gem", // RubyGems publishing
	// Rust
	"katyo/publish-crates", // crates.io publishing
	"obi1kenobi/cargo-semver-checks-action", // often paired with publish
	// .NET
	"nuget/setup-nuget", // often precedes nuget push
	// Java
	"gradle/gradle-build-action", // when used with publish task
	// Homebrew
	"homebrew/actions", // Homebrew formula updates
	"dawidd6/action-homebrew-bump-formula", // Homebrew formula bumps

	// === Container Registries ===
	"docker/build-push-action", // Docker Hub, GHCR, ECR, etc.
	"docker/login-action",      // Often precedes push
	"docker/metadata-action",   // Often precedes push
	"aws-actions/amazon-ecr-login", // ECR login
	"google-github-actions/setup-gcloud", // GCR setup
	"azure/docker-login",       // ACR login

	// === Cloud Platforms ===
	// AWS
	"aws-actions/configure-aws-credentials", // AWS access
	"aws-actions/amazon-ecs-deploy-task-definition", // ECS deploy
	"aws-actions/amazon-ecs-render-task-definition", // ECS render
	"aws-actions/aws-cloudformation-github-deploy", // CloudFormation
	// GCP
	"google-github-actions/deploy-cloudrun", // Cloud Run
	"google-github-actions/deploy-appengine", // App Engine
	"google-github-actions/get-gke-credentials", // GKE access
	"google-github-actions/deploy-cloud-functions", // Cloud Functions
	"google-github-actions/upload-cloud-storage", // GCS upload
	// Azure
	"azure/webapps-deploy",    // Azure Web Apps
	"azure/functions-action",  // Azure Functions
	"azure/aks-set-context",   // AKS access
	"azure/k8s-deploy",        // Kubernetes deploy
	"azure/container-apps-deploy-action", // Container Apps
	// Heroku
	"akhileshns/heroku-deploy", // Heroku deployment
	// Vercel
	"amondnet/vercel-action",   // Vercel deployment
	"vercel/action",            // Official Vercel action
	// Netlify
	"netlify/actions/deploy",   // Netlify deployment
	"nwtgck/actions-netlify",   // Netlify deployment
	// Cloudflare
	"cloudflare/wrangler-action", // Cloudflare Workers
	"cloudflare/pages-action",    // Cloudflare Pages
	// Railway
	"railwayapp/railway-action", // Railway deployment
	// Fly.io
	"superfly/flyctl-actions",   // Fly.io deployment
	// Render
	"render-oss/render-deploy-action", // Render deployment
	// DigitalOcean
	"digitalocean/action-doctl", // DigitalOcean CLI

	// === Static Hosting ===
	"jamesives/github-pages-deploy-action", // GH Pages
	"peaceiris/actions-gh-pages",   // GH Pages
	"firebase/firebase-tools",      // Firebase Hosting
	"FirebaseExtended/action-hosting-deploy", // Firebase Hosting
	"w9jds/firebase-action",        // Firebase (general)

	// === Kubernetes ===
	"azure/k8s-set-context",        // K8s context
	"azure/k8s-create-secret",      // K8s secrets
	"helm/chart-releaser-action",   // Helm chart releases
	"deliverybot/helm",             // Helm deployments
	"koslib/helm-eks-action",       // Helm on EKS

	// === Infrastructure as Code ===
	"hashicorp/setup-terraform", // Terraform (often precedes apply)
	"pulumi/actions",            // Pulumi deployments
	"aws-actions/aws-cdk",       // CDK deployments

	// === Serverless ===
	"serverless/github-action",  // Serverless Framework
	"aws-actions/aws-lambda-action", // Lambda deploys

	// === GitHub Releases ===
	"softprops/action-gh-release", // GitHub Releases
	"ncipollo/release-action",     // GitHub Releases
	"marvinpinto/action-automatic-releases", // Auto releases
}

// sensitiveCommands contains shell commands that perform publishing or deployment.
// Jobs with run: steps containing these should NOT get if: always().
// This list is intentionally comprehensive to err on the side of safety.
var sensitiveCommands = []string{
	// === Package Managers ===
	// JavaScript/Node.js
	"npm publish", "yarn publish", "pnpm publish",
	"npm dist-tag", "yarn npm publish",
	"npx semantic-release", "npx changeset publish",
	// Python
	"twine upload", "python -m twine", "python3 -m twine",
	"poetry publish", "flit publish", "pdm publish",
	"pip upload", // rare but possible
	// Ruby
	"gem push", "gem release", "rake release",
	"bundle exec rake release",
	// Rust
	"cargo publish",
	// Go
	"goreleaser release", "goreleaser build --snapshot=false",
	// .NET
	"dotnet nuget push", "nuget push", "dotnet pack && dotnet nuget",
	// Java/Kotlin
	"mvn deploy", "mvn release:perform",
	"gradle publish", "gradle publishToMaven",
	"./gradlew publish", "./mvnw deploy",
	// PHP
	"composer publish", // rare, usually via Packagist
	// Elixir
	"mix hex.publish",
	// Dart/Flutter
	"dart pub publish", "flutter pub publish",
	// Swift/Cocoapods
	"pod trunk push", "pod lib lint && pod trunk",

	// === Container Registries ===
	"docker push", "docker buildx push",
	"docker-compose push", "docker compose push",
	"podman push", "buildah push",
	"crane push", "skopeo copy", // OCI tools
	// AWS ECR
	"aws ecr get-login", "docker login -u AWS",
	// GCR
	"docker push gcr.io", "docker push us.gcr.io",
	"docker push eu.gcr.io", "docker push asia.gcr.io",
	// Azure ACR
	"az acr login", "docker push .azurecr.io",
	// GHCR
	"docker push ghcr.io",

	// === Git Operations ===
	"git push --tags", "git push origin refs/tags",
	"git push origin --tags", "git tag -a && git push",
	"git push --follow-tags",

	// === GitHub CLI ===
	"gh release create", "gh release upload",
	"gh release edit", "gh pr merge --auto",

	// === Kubernetes ===
	"kubectl apply", "kubectl create", "kubectl replace",
	"kubectl set image", "kubectl rollout",
	"kubectl patch", "kubectl scale",
	// Destructive operations
	"kubectl delete", "kubectl drain",
	// Kustomize
	"kubectl apply -k", "kustomize build | kubectl apply",

	// === Helm ===
	"helm install", "helm upgrade", "helm push",
	"helm package && helm push",
	// Destructive operations
	"helm delete", "helm uninstall", "helm rollback",

	// === Terraform ===
	"terraform apply", "terraform destroy",
	"terraform import",
	"tofu apply", "tofu destroy", // OpenTofu
	// Terragrunt
	"terragrunt apply", "terragrunt destroy",
	"terragrunt run-all apply",

	// === Pulumi ===
	"pulumi up", "pulumi update", "pulumi destroy",
	"pulumi preview --diff", // only if followed by up

	// === AWS CDK ===
	"cdk deploy", "cdk destroy",
	"npx cdk deploy", "npx aws-cdk deploy",

	// === Cloud CLIs ===
	// AWS
	"aws s3 sync", "aws s3 cp", "aws s3 mv", "aws s3 rm",
	"aws s3api put-object",
	"aws lambda update-function", "aws lambda publish",
	"aws ecs update-service", "aws ecs deploy",
	"aws cloudformation deploy", "aws cloudformation create-stack",
	"aws cloudformation update-stack",
	"aws elasticbeanstalk update-environment",
	"aws amplify start-deployment",
	"sam deploy", "sam package && sam deploy",
	// GCP
	"gcloud app deploy", "gcloud run deploy",
	"gcloud functions deploy", "gcloud compute deploy",
	"gcloud builds submit", // when used with deploy
	"gcloud container clusters",
	// Azure
	"az webapp deploy", "az functionapp deploy",
	"az acr build", "az aks update",
	"az container create", "az container app up",

	// === Platform-as-a-Service ===
	// Heroku
	"heroku deploy", "heroku releases:create",
	"heroku container:release", "heroku container:push",
	"git push heroku",
	// Fly.io
	"flyctl deploy", "fly deploy", "fly launch",
	"flyctl machine run",
	// Railway
	"railway deploy", "railway up",
	// Render
	"render deploy",
	// Vercel
	"vercel --prod", "vercel deploy --prod",
	"vercel --production", "vercel deploy --production",
	// Netlify
	"netlify deploy --prod", "netlify deploy --production",
	// Cloudflare
	"wrangler publish", "wrangler deploy",
	"npx wrangler publish", "npx wrangler deploy",
	// DigitalOcean
	"doctl apps create-deployment",
	"doctl kubernetes cluster",
	// Dokku
	"dokku deploy", "git push dokku",
	// Platform.sh
	"platform deploy", "platform push",
	// Aptible
	"aptible deploy",

	// === Serverless ===
	"serverless deploy", "sls deploy",
	"npx serverless deploy", "npx sls deploy",
	"firebase deploy", "firebase hosting:channel:deploy",
	"amplify publish", "amplify push",

	// === Database Migrations ===
	// These can cause production data changes
	"flyway migrate", "flyway repair",
	"liquibase update", "liquibase rollback",
	"alembic upgrade", "alembic downgrade",
	"knex migrate:latest", "knex migrate:rollback",
	"prisma migrate deploy", "prisma db push",
	"prisma migrate reset", // destructive
	"django-admin migrate", "python manage.py migrate",
	"rails db:migrate", "rake db:migrate",
	"bundle exec rails db:migrate",
	"sequelize db:migrate",
	"typeorm migration:run",
	"goose up", "goose down",
	"dbmate up", "dbmate down",
	"atlas migrate apply", "atlas schema apply",

	// === SSH/Remote Deployment ===
	"ssh .* && ", // SSH with command chaining
	"rsync -avz", // when used for deployment
	"scp ", // file transfers to servers
	"ansible-playbook", // Ansible deployments
	"fabric deploy", "fab deploy",
	"capistrano deploy", "cap deploy",
}

// Package-level sets for O(1) substring lookups in IsSensitiveJob.
// These are built once at init time from the original arrays.
var (
	sensitiveJobNamesSet  = buildStringSet(sensitiveJobNames)
	sensitiveActionsSet   = buildStringSet(sensitiveActions)
	sensitiveCommandsSet  = buildStringSet(sensitiveCommands)
)

// containsSensitiveSubstring checks if haystack contains any key from the set as a substring.
// This is optimized for the common case where we need to check multiple patterns.
func containsSensitiveSubstring(haystack string, patterns map[string]struct{}) bool {
	for pattern := range patterns {
		if strings.Contains(haystack, pattern) {
			return true
		}
	}
	return false
}

// IsSensitiveJob returns true if the job might publish, release, or deploy.
// These jobs should NOT get if: always() to prevent accidental production releases.
func IsSensitiveJob(jobID string, job *Job) bool {
	if job == nil {
		return false
	}

	// Check job ID and name for sensitive keywords
	// Cache the lowercase result to avoid repeated conversions
	jobNameLower := strings.ToLower(jobID)
	if job.Name != "" {
		jobNameLower = strings.ToLower(job.Name)
	}

	if containsSensitiveSubstring(jobNameLower, sensitiveJobNamesSet) {
		return true
	}

	// Check steps for sensitive actions or commands
	for _, step := range job.Steps {
		if step == nil {
			continue
		}

		// Check for publishing/deployment actions
		if step.Uses != "" {
			// Cache lowercase conversion for this step
			actionLower := strings.ToLower(step.Uses)

			// Check known dangerous actions using the set
			if containsSensitiveSubstring(actionLower, sensitiveActionsSet) {
				return true
			}

			// Check generic patterns in action names
			if strings.Contains(actionLower, "/deploy") ||
				strings.Contains(actionLower, "/publish") ||
				strings.Contains(actionLower, "/release") ||
				strings.Contains(actionLower, "-deploy") ||
				strings.Contains(actionLower, "-publish") ||
				strings.Contains(actionLower, "-release") {
				return true
			}
		}

		// Check run commands for publishing/deployment
		if step.Run != "" {
			// Cache lowercase conversion for this step
			cmdLower := strings.ToLower(step.Run)

			if containsSensitiveSubstring(cmdLower, sensitiveCommandsSet) {
				return true
			}
		}
	}

	return false
}

// InjectAlwaysForDependentJobs injects if: always() for jobs with dependencies.
// This ensures dependent jobs run even if their dependencies fail, allowing
// Detent to capture ALL errors instead of stopping at the first failure.
// Jobs with existing if: conditions get them combined: if: always() && (original)
//
// Job override states:
//   - "skip": Force job to skip by injecting if: false
//   - "run": Force job to run (bypass security check, inject if: always())
//   - "" (auto): Default behavior - skip sensitive jobs, run others
//
// Parameters:
//   - wf: The workflow to modify
//   - jobOverrides: Map of jobID -> state ("run", "skip", or "" for auto).
//     Pass nil to use auto behavior for all jobs.
func InjectAlwaysForDependentJobs(wf *Workflow, jobOverrides map[string]string) {
	if wf == nil || wf.Jobs == nil {
		return
	}

	for jobID, job := range wf.Jobs {
		if job == nil {
			continue
		}

		// Skip reusable workflows (they don't support if: at job level)
		if job.Uses != "" {
			continue
		}

		override := jobOverrides[jobID]

		switch override {
		case "skip":
			// Force skip: inject if: false
			job.If = "false"
			continue
		case "run":
			// Force run: fall through to inject if: always()
		default:
			// Auto: skip sensitive jobs (no injection)
			if IsSensitiveJob(jobID, job) {
				continue
			}
			// Also skip jobs without dependencies for auto mode
			if !jobHasNeeds(job) {
				continue
			}
		}

		// Combine with existing condition if present
		if job.If != "" {
			// Wrap to preserve operator precedence
			job.If = fmt.Sprintf("always() && (%s)", job.If)
		} else {
			job.If = "always()"
		}
	}
}

const (
	// Display limits for step names derived from run commands
	maxRunCommandDisplay = 40 // Max length before truncation
	truncationSuffix     = "..."

	// Concurrency limit for parallel workflow processing
	maxConcurrentWorkflows = 10
)

// truncatedRunLength is derived from maxRunCommandDisplay to ensure the truncated
// string plus suffix equals exactly maxRunCommandDisplay characters.
var truncatedRunLength = maxRunCommandDisplay - len(truncationSuffix)

// Environment variable names for timeout configuration.
const (
	// JobTimeoutEnv overrides the default job timeout.
	// Value should be in minutes (e.g., "60" for 60 minutes).
	JobTimeoutEnv = "DETENT_JOB_TIMEOUT"

	// StepTimeoutEnv overrides the default step timeout.
	// Value should be in minutes (e.g., "20" for 20 minutes).
	StepTimeoutEnv = "DETENT_STEP_TIMEOUT"
)

// Default timeout values in minutes.
const (
	defaultJobTimeoutMinutes  = 30
	defaultStepTimeoutMinutes = 15

	// Minimum and maximum allowed timeout values (in minutes).
	minTimeoutMinutes = 1
	maxTimeoutMinutes = 120
)

// getJobTimeout returns the default job timeout in minutes.
// Reads from DETENT_JOB_TIMEOUT, defaults to 30 minutes.
func getJobTimeout() int {
	return getTimeoutFromEnv(JobTimeoutEnv, defaultJobTimeoutMinutes)
}

// getStepTimeout returns the default step timeout in minutes.
// Reads from DETENT_STEP_TIMEOUT, defaults to 15 minutes.
func getStepTimeout() int {
	return getTimeoutFromEnv(StepTimeoutEnv, defaultStepTimeoutMinutes)
}

// getTimeoutFromEnv reads a timeout value from an environment variable.
// Returns the default if the env var is not set, empty, or invalid.
// Values are clamped to [minTimeoutMinutes, maxTimeoutMinutes].
func getTimeoutFromEnv(envVar string, defaultValue int) int {
	value := os.Getenv(envVar)
	if value == "" {
		return defaultValue
	}

	minutes, err := strconv.Atoi(value)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: invalid %s value %q, using default %d minutes\n",
			envVar, value, defaultValue)
		return defaultValue
	}

	return clampTimeout(minutes, defaultValue)
}

// clampTimeout ensures the timeout is within valid bounds.
// Returns the default if the value is out of range.
func clampTimeout(minutes, defaultValue int) int {
	if minutes < minTimeoutMinutes {
		fmt.Fprintf(os.Stderr, "warning: timeout %d minutes is below minimum %d, using default %d minutes\n",
			minutes, minTimeoutMinutes, defaultValue)
		return defaultValue
	}
	if minutes > maxTimeoutMinutes {
		fmt.Fprintf(os.Stderr, "warning: timeout %d minutes exceeds maximum %d, using default %d minutes\n",
			minutes, maxTimeoutMinutes, defaultValue)
		return defaultValue
	}
	return minutes
}

// InjectTimeouts adds reasonable timeout values to prevent hanging Docker operations.
// Jobs default to 30 minutes, steps to 15 minutes. Only applied if not already set.
// Timeouts can be configured via DETENT_JOB_TIMEOUT and DETENT_STEP_TIMEOUT environment variables.
func InjectTimeouts(wf *Workflow) {
	if wf == nil || wf.Jobs == nil {
		return
	}

	jobTimeout := getJobTimeout()
	stepTimeout := getStepTimeout()

	for _, job := range wf.Jobs {
		if job == nil {
			continue
		}

		// Set job timeout if not already specified
		if job.TimeoutMinutes == nil {
			job.TimeoutMinutes = jobTimeout
		}

		// Set step timeouts if not already specified
		if job.Steps != nil {
			for _, step := range job.Steps {
				if step == nil {
					continue
				}
				if step.TimeoutMinutes == nil {
					step.TimeoutMinutes = stepTimeout
				}
			}
		}
	}
}

// BuildManifest creates a v2 manifest from a workflow containing full job and step information.
// The manifest includes job IDs, display names, step names, dependencies, and reusable workflow references.
// Jobs are returned in topological order (respecting dependencies).
func BuildManifest(wf *Workflow) *ci.ManifestInfo {
	if wf == nil || wf.Jobs == nil {
		return &ci.ManifestInfo{Version: 2, Jobs: []ci.ManifestJob{}}
	}

	// Build job info map for topological sorting
	jobInfoMap := make(map[string]*ci.ManifestJob)
	for jobID, job := range wf.Jobs {
		if job == nil || !isValidJobID(jobID) {
			continue
		}

		mj := &ci.ManifestJob{
			ID:        jobID,
			Name:      job.Name,
			Sensitive: IsSensitiveJob(jobID, job),
		}
		if mj.Name == "" {
			mj.Name = jobID
		}

		// Handle reusable workflows
		if job.Uses != "" {
			mj.Uses = job.Uses
		} else {
			// Extract step names
			for _, step := range job.Steps {
				stepName := getStepDisplayName(step)
				mj.Steps = append(mj.Steps, stepName)
			}
		}

		// Parse dependencies
		mj.Needs = parseJobNeeds(job.Needs)

		jobInfoMap[jobID] = mj
	}

	// Topological sort for consistent ordering
	sortedJobs := topologicalSortManifest(jobInfoMap)

	return &ci.ManifestInfo{
		Version: 2,
		Jobs:    sortedJobs,
	}
}

// BuildCombinedManifest builds a single manifest from multiple workflows.
// This ensures all jobs from all workflow files are included in one manifest,
// which is injected once for consistent TUI display.
func BuildCombinedManifest(workflows map[string]*Workflow) *ci.ManifestInfo {
	if len(workflows) == 0 {
		return &ci.ManifestInfo{Version: 2, Jobs: []ci.ManifestJob{}}
	}

	// Sort workflow paths for deterministic ordering
	paths := make([]string, 0, len(workflows))
	for p := range workflows {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	// Collect all jobs from all workflows
	allJobsMap := make(map[string]*ci.ManifestJob)
	for _, path := range paths {
		wf := workflows[path]
		wfManifest := BuildManifest(wf)
		for i := range wfManifest.Jobs {
			job := &wfManifest.Jobs[i]
			allJobsMap[job.ID] = job
		}
	}

	// Topological sort for consistent ordering
	sortedJobs := topologicalSortManifest(allJobsMap)

	return &ci.ManifestInfo{
		Version: 2,
		Jobs:    sortedJobs,
	}
}

// findFirstJobAcrossWorkflows finds the first valid job ID and its workflow path
// across all workflows. Prefers jobs WITHOUT dependencies (needs:) since those
// run first. Among valid candidates, picks alphabetically first.
func findFirstJobAcrossWorkflows(workflows map[string]*Workflow) (workflowPath, jobID string) {
	// Sort workflow paths for deterministic ordering
	paths := make([]string, 0, len(workflows))
	for p := range workflows {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	// First pass: find jobs without dependencies (they run first)
	var bestPath, bestJobID string
	var fallbackPath, fallbackJobID string // Fallback if all jobs have dependencies

	for _, path := range paths {
		wf := workflows[path]
		if wf == nil || wf.Jobs == nil {
			continue
		}

		for jID, job := range wf.Jobs {
			if job == nil || job.Uses != "" || !isValidJobID(jID) {
				continue
			}

			// Track fallback (any valid job, alphabetically first)
			if fallbackJobID == "" || path < fallbackPath || (path == fallbackPath && jID < fallbackJobID) {
				fallbackPath = path
				fallbackJobID = jID
			}

			// Prefer jobs without dependencies
			if !jobHasNeeds(job) {
				if bestJobID == "" || path < bestPath || (path == bestPath && jID < bestJobID) {
					bestPath = path
					bestJobID = jID
				}
			}
		}
	}

	// Return job without dependencies if found, otherwise fallback
	if bestJobID != "" {
		return bestPath, bestJobID
	}
	return fallbackPath, fallbackJobID
}

// jobHasNeeds returns true if the job has dependencies (needs field).
func jobHasNeeds(job *Job) bool {
	if job == nil || job.Needs == nil {
		return false
	}
	switch v := job.Needs.(type) {
	case string:
		return v != ""
	case []any:
		return len(v) > 0
	case []string:
		return len(v) > 0
	}
	return false
}

// getStepDisplayName returns a human-readable name for a step.
// Tries: step.Name, step.ID, action name from step.Uses, truncated run command.
func getStepDisplayName(step *Step) string {
	if step == nil {
		return "Unknown step"
	}
	if step.Name != "" {
		return step.Name
	}
	if step.ID != "" {
		return step.ID
	}
	if step.Uses != "" {
		// Extract action name from "owner/repo@ref" or "owner/repo/path@ref"
		parts := strings.Split(step.Uses, "@")
		if len(parts) > 0 {
			actionPath := parts[0]
			segments := strings.Split(actionPath, "/")
			if len(segments) >= 2 {
				return segments[len(segments)-1] // Last segment is action name
			}
			return actionPath
		}
		return step.Uses
	}
	if step.Run != "" {
		// Truncate long run commands
		run := strings.TrimSpace(step.Run)
		run = strings.Split(run, "\n")[0] // First line only
		if len(run) > maxRunCommandDisplay {
			return run[:truncatedRunLength] + truncationSuffix
		}
		return run
	}
	return "Step"
}

// parseJobNeeds extracts job dependencies from the needs field.
// Handles both string and []string formats.
func parseJobNeeds(needs any) []string {
	if needs == nil {
		return nil
	}

	switch v := needs.(type) {
	case string:
		if v != "" {
			return []string{v}
		}
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				result = append(result, s)
			}
		}
		return result
	case []string:
		return v
	}
	return nil
}

// topologicalSortManifest sorts jobs by dependencies (jobs with fewer deps first).
func topologicalSortManifest(jobInfoMap map[string]*ci.ManifestJob) []ci.ManifestJob {
	// Build in-degree map
	inDegree := make(map[string]int)
	for id := range jobInfoMap {
		inDegree[id] = 0
	}
	for _, job := range jobInfoMap {
		for _, need := range job.Needs {
			if _, exists := jobInfoMap[need]; exists {
				inDegree[job.ID]++
			}
		}
	}

	// Build reverse dependency graph: dependents[jobID] = list of jobs that depend on jobID
	dependents := make(map[string][]string)
	for id := range jobInfoMap {
		dependents[id] = nil
	}
	for _, job := range jobInfoMap {
		for _, need := range job.Needs {
			if _, exists := jobInfoMap[need]; exists {
				dependents[need] = append(dependents[need], job.ID)
			}
		}
	}

	// Kahn's algorithm with stable sorting
	result := make([]ci.ManifestJob, 0, len(jobInfoMap))
	var queue []string

	// Start with jobs that have no dependencies
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}
	sort.Strings(queue) // Stable order

	for len(queue) > 0 {
		// Pop first item
		current := queue[0]
		queue = queue[1:]

		if job, exists := jobInfoMap[current]; exists {
			result = append(result, *job)
		}

		// Use pre-computed dependents for O(1) lookup
		var nextBatch []string
		for _, dependent := range dependents[current] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				nextBatch = append(nextBatch, dependent)
			}
		}
		sort.Strings(nextBatch)
		queue = append(queue, nextBatch...)
	}

	// Add any remaining jobs (cycles or missing deps)
	if len(result) < len(jobInfoMap) {
		var remaining []string
		added := make(map[string]bool)
		for _, job := range result {
			added[job.ID] = true
		}
		for id := range jobInfoMap {
			if !added[id] {
				remaining = append(remaining, id)
			}
		}
		sort.Strings(remaining)
		for _, id := range remaining {
			if job, exists := jobInfoMap[id]; exists {
				result = append(result, *job)
			}
		}
	}

	return result
}

// InjectJobMarkers injects lifecycle marker steps into each job for reliable job tracking.
// This uses the v2 manifest format with full step information.
// Each job gets:
// - A manifest step (first job only, contains all job/step info as JSON)
// - Step-start markers before each user step
// - A job-end marker with always() condition
// Jobs using reusable workflows (uses:) are skipped as they have no steps to inject.
// Jobs with invalid IDs (not matching GitHub Actions spec) are skipped for security.
//
// Note: For multi-workflow scenarios, use InjectJobMarkersWithManifest to share a combined manifest.
func InjectJobMarkers(wf *Workflow) {
	if wf == nil || wf.Jobs == nil {
		return
	}

	// Build the v2 manifest for this single workflow
	manifest := BuildManifest(wf)
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		// FALLBACK: If marshaling fails (should be rare), use empty manifest.
		// This allows workflow execution to continue without job tracking info.
		// The TUI will show "unknown" for job names but execution will work.
		manifestJSON = []byte(`{"v":2,"jobs":[]}`)
	}

	// Find the first job alphabetically to inject manifest
	var firstJobID string
	for jobID, job := range wf.Jobs {
		if job == nil || job.Uses != "" || !isValidJobID(jobID) {
			continue
		}
		if firstJobID == "" || jobID < firstJobID {
			firstJobID = jobID
		}
	}

	injectJobMarkersInternal(wf, manifestJSON, firstJobID)
}

// InjectJobMarkersWithManifest injects lifecycle markers using an externally-provided manifest.
// Use this when processing multiple workflows to ensure all jobs appear in a single manifest.
// Parameters:
//   - wf: The workflow to inject markers into
//   - manifestJSON: Pre-built manifest JSON (nil to skip manifest injection for this workflow)
//   - manifestJobID: The job ID that should receive the manifest step (empty string to skip)
func InjectJobMarkersWithManifest(wf *Workflow, manifestJSON []byte, manifestJobID string) {
	if wf == nil || wf.Jobs == nil {
		return
	}
	injectJobMarkersInternal(wf, manifestJSON, manifestJobID)
}

// injectJobMarkersInternal is the shared implementation for marker injection.
func injectJobMarkersInternal(wf *Workflow, manifestJSON []byte, manifestJobID string) {
	for jobID, job := range wf.Jobs {
		if job == nil {
			continue
		}

		// Skip reusable workflows (they have no steps to inject)
		if job.Uses != "" {
			continue
		}

		// Skip jobs with invalid IDs to prevent shell injection
		if !isValidJobID(jobID) {
			continue
		}

		var newSteps []*Step

		// Add manifest step only to the designated job
		// Use base64 encoding to prevent shell injection from manifest JSON content
		if manifestJSON != nil && jobID == manifestJobID {
			encoded := base64.StdEncoding.EncodeToString(manifestJSON)
			manifestStep := &Step{
				Name: "detent: manifest",
				Run:  fmt.Sprintf("echo '::detent::manifest::v2::b64::%s'", encoded),
			}
			newSteps = append(newSteps, manifestStep)
		}

		// Add job-start marker
		jobStartStep := &Step{
			Name: "detent: job start",
			Run:  fmt.Sprintf("echo '::detent::job-start::%s'", jobID),
		}
		newSteps = append(newSteps, jobStartStep)

		// Add step markers before each original step
		for i, step := range job.Steps {
			stepName := getStepDisplayName(step)
			markerStep := &Step{
				Name: fmt.Sprintf("detent: step %d", i),
				Run:  fmt.Sprintf("echo '::detent::step-start::%s::%d::%s'", jobID, i, sanitizeForShellEcho(stepName)),
			}
			newSteps = append(newSteps, markerStep, step)
		}

		// Add job end marker with always() to capture success/failure/cancelled
		endStep := &Step{
			Name: "detent: job end",
			If:   "always()",
			Run:  fmt.Sprintf("echo '::detent::job-end::%s::${{ job.status }}'", jobID),
		}
		newSteps = append(newSteps, endStep)

		job.Steps = newSteps
	}
}

// sanitizeForShellEcho sanitizes a string for safe use in a single-quoted echo command.
// This handles all shell metacharacters that could break single-quoted strings or
// allow command injection:
//   - Replaces newlines and tabs with spaces (prevents breaking the echo command)
//   - Escapes single quotes using the '\'' pattern (end quote, escaped quote, start quote)
//   - Removes null bytes (could truncate the string in shell)
func sanitizeForShellEcho(s string) string {
	// Replace control characters that could break the echo command
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\x00", "") // Remove null bytes

	// Escape single quotes for safe use in single-quoted strings
	// 'foo'\''bar' = foo'bar (end quote, escaped quote, start quote)
	s = strings.ReplaceAll(s, "'", "'\\''")

	return s
}

// isValidJobID checks if a job ID matches GitHub Actions requirements.
// Valid IDs must start with a letter or underscore and contain only alphanumeric, underscore, or hyphen.
// This validation prevents shell injection in marker echo commands.
func isValidJobID(jobID string) bool {
	return validJobIDPattern.MatchString(jobID)
}

// PrepareWorkflows processes workflows and returns temp directory path.
// If specificWorkflow is provided, only that workflow is processed.
// Otherwise, all workflows in srcDir are discovered and processed.
//
// Parameters:
//   - srcDir: The directory containing workflow files
//   - specificWorkflow: Optional specific workflow file to process (empty for all)
//   - jobOverrides: Map of jobID -> state ("run", "skip", or "" for auto).
//     Pass nil to use auto behavior for all jobs.
func PrepareWorkflows(srcDir, specificWorkflow string, jobOverrides map[string]string) (tmpDir string, cleanup func(), err error) {
	var workflows []string

	if specificWorkflow != "" {
		// Validate path BEFORE cleaning to catch patterns like ./file
		if filepath.IsAbs(specificWorkflow) || specificWorkflow != "" && specificWorkflow[0] == '.' {
			return "", nil, fmt.Errorf("workflow path must be relative and cannot reference parent directories")
		}

		// Clean the path after validation
		cleanWorkflow := filepath.Clean(specificWorkflow)

		// Get absolute paths for validation
		absSrcDir, absErr := filepath.Abs(srcDir)
		if absErr != nil {
			return "", nil, fmt.Errorf("resolving source directory: %w", absErr)
		}

		// Process specific workflow file
		workflowPath := filepath.Join(absSrcDir, cleanWorkflow)
		absPath, absPathErr := filepath.Abs(workflowPath)
		if absPathErr != nil {
			return "", nil, fmt.Errorf("resolving workflow path: %w", absPathErr)
		}

		// Validate the resolved path is within the source directory using filepath.Rel
		relPath, relErr := filepath.Rel(absSrcDir, absPath)
		if relErr != nil || strings.HasPrefix(relPath, "..") {
			return "", nil, fmt.Errorf("workflow path must be within the workflows directory")
		}

		// Validate file exists and is a workflow file
		fileInfo, statErr := os.Lstat(absPath)
		if statErr != nil {
			return "", nil, fmt.Errorf("workflow file not found: %w", statErr)
		}

		// Reject symlinks to prevent path traversal
		if fileInfo.Mode()&os.ModeSymlink != 0 {
			return "", nil, fmt.Errorf("workflow file cannot be a symlink")
		}

		ext := filepath.Ext(cleanWorkflow)
		if ext != ".yml" && ext != ".yaml" {
			return "", nil, fmt.Errorf("workflow file must have .yml or .yaml extension")
		}

		workflows = []string{absPath}
	} else {
		// Discover all workflows
		workflows, err = DiscoverWorkflows(srcDir)
		if err != nil {
			return "", nil, err
		}

		if len(workflows) == 0 {
			return "", nil, fmt.Errorf("no workflow files found in %s", srcDir)
		}
	}

	// First pass: parse and validate all workflows before creating temp directory
	parsedWorkflows := make(map[string]*Workflow, len(workflows))
	for _, wfPath := range workflows {
		wf, parseErr := ParseWorkflowFile(wfPath)
		if parseErr != nil {
			return "", nil, fmt.Errorf("parsing %s: %w", wfPath, parseErr)
		}
		parsedWorkflows[wfPath] = wf
	}

	// Validate all workflows for unsupported features
	var allWorkflows []*Workflow
	for _, wf := range parsedWorkflows {
		allWorkflows = append(allWorkflows, wf)
	}
	if validationErr := ValidateWorkflows(allWorkflows); validationErr != nil {
		// Only block on actual errors, not warnings
		if validationErrors, ok := validationErr.(ValidationErrors); ok {
			if validationErrors.HasErrors() {
				return "", nil, validationErrors.Errors()
			}
			// Warnings only - continue execution (warnings are logged elsewhere if needed)
		} else {
			return "", nil, validationErr
		}
	}

	tmpDir, err = os.MkdirTemp("", "detent-workflows-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating temp directory: %w", err)
	}

	// Explicitly set restrictive permissions (defense in depth)
	// MkdirTemp already uses 0700 on most systems, but this ensures consistency
	//nolint:gosec // G302: 0o700 is correct for directories (execute bit needed to traverse)
	if chmodErr := os.Chmod(tmpDir, 0o700); chmodErr != nil {
		_ = os.RemoveAll(tmpDir)
		return "", nil, fmt.Errorf("setting temp directory permissions: %w", chmodErr)
	}

	cleanup = func() {
		_ = os.RemoveAll(tmpDir)
	}

	// Build combined manifest from ALL workflows before processing
	// This ensures the TUI sees all jobs from all workflow files in a single manifest
	combinedManifest := BuildCombinedManifest(parsedWorkflows)
	combinedManifestJSON, err := json.Marshal(combinedManifest)
	if err != nil {
		// FALLBACK: If marshaling fails (should be rare), use empty manifest.
		// This allows workflow execution to continue without job tracking info.
		// The TUI will show "unknown" for job names but execution will work.
		combinedManifestJSON = []byte(`{"v":2,"jobs":[]}`)
	}

	// Find which workflow and job should receive the manifest
	manifestWfPath, manifestJobID := findFirstJobAcrossWorkflows(parsedWorkflows)

	// Process workflows in parallel using errgroup
	// Each workflow is independent, so parallel processing is safe
	var g errgroup.Group
	var mu sync.Mutex // Protects file writes to tmpDir

	// Set a reasonable concurrency limit to avoid resource exhaustion
	g.SetLimit(maxConcurrentWorkflows)

	for wfPath, wf := range parsedWorkflows {
		wfPath := wfPath // Capture loop variable for goroutine
		wf := wf         // Capture loop variable for goroutine
		g.Go(func() error {
			// Apply modifications
			// Order matters: continue-on-error first, then always() for deps, then markers, then timeouts
			InjectContinueOnError(wf)
			InjectAlwaysForDependentJobs(wf, jobOverrides)

			// Inject markers with combined manifest (only first workflow gets manifest step)
			if wfPath == manifestWfPath {
				InjectJobMarkersWithManifest(wf, combinedManifestJSON, manifestJobID)
			} else {
				// Other workflows get markers but no manifest
				InjectJobMarkersWithManifest(wf, nil, "")
			}

			InjectTimeouts(wf)

			// Marshal to YAML
			data, marshalErr := yaml.Marshal(wf)
			if marshalErr != nil {
				return fmt.Errorf("marshaling %s: %w", wfPath, marshalErr)
			}

			// Write to temp directory (mutex-protected to ensure thread-safe file writes)
			filename := filepath.Base(wfPath)
			mu.Lock()
			writeErr := os.WriteFile(filepath.Join(tmpDir, filename), data, 0o600)
			mu.Unlock()

			if writeErr != nil {
				return fmt.Errorf("writing %s: %w", filename, writeErr)
			}

			return nil
		})
	}

	// Wait for all goroutines to complete and check for errors
	if err := g.Wait(); err != nil {
		cleanup()
		return "", nil, err
	}

	return tmpDir, cleanup, nil
}
