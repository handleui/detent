import type { Job } from "./types.js";

/**
 * Keywords that indicate a job may publish, release, or deploy.
 * Jobs containing these keywords should NOT get `if: always()` to prevent accidental production releases.
 */
const sensitiveJobNames = [
  // Core deployment/release terms
  "release",
  "publish",
  "deploy",
  "production",
  "prod",
  "staging",
  "ship",
  "distribute",
  "upload",
  // Additional deployment contexts
  "live",
  "canary",
  "rollout",
  "blue-green",
  "bluegreen",
  "promote",
  "delivery",
  "push-to",
  "push_to",
  // Infrastructure and migration terms
  "infra",
  "migration",
  "migrate",
  "scale",
  "provision",
  // Platform-specific terms
  "npm-publish",
  "docker-push",
  "pypi",
  "rubygems",
  "nuget",
  "homebrew",
  "brew-",
  "cargo-publish",
  "maven-deploy",
];

/**
 * GitHub Actions that perform publishing or deployment.
 * Jobs using these actions should NOT get `if: always()` to prevent accidental production releases.
 */
const sensitiveActions = [
  // === Package Publishing ===
  // JavaScript/Node.js
  "changesets/action",
  "JS-DevTools/npm-publish",
  "primer/publish",
  // Go
  "goreleaser/goreleaser-action",
  // Python
  "pypa/gh-action-pypi-publish",
  // Ruby
  "rubygems/release-gem",
  // Rust
  "katyo/publish-crates",
  "obi1kenobi/cargo-semver-checks-action",
  // .NET
  "nuget/setup-nuget",
  // Java
  "gradle/gradle-build-action",
  // Homebrew
  "homebrew/actions",
  "dawidd6/action-homebrew-bump-formula",

  // === Container Registries ===
  "docker/build-push-action",
  "docker/login-action",
  "docker/metadata-action",
  "aws-actions/amazon-ecr-login",
  "google-github-actions/setup-gcloud",
  "azure/docker-login",

  // === Cloud Platforms ===
  // AWS
  "aws-actions/configure-aws-credentials",
  "aws-actions/amazon-ecs-deploy-task-definition",
  "aws-actions/amazon-ecs-render-task-definition",
  "aws-actions/aws-cloudformation-github-deploy",
  // GCP
  "google-github-actions/deploy-cloudrun",
  "google-github-actions/deploy-appengine",
  "google-github-actions/get-gke-credentials",
  "google-github-actions/deploy-cloud-functions",
  "google-github-actions/upload-cloud-storage",
  // Azure
  "azure/webapps-deploy",
  "azure/functions-action",
  "azure/aks-set-context",
  "azure/k8s-deploy",
  "azure/container-apps-deploy-action",
  // Heroku
  "akhileshns/heroku-deploy",
  // Vercel
  "amondnet/vercel-action",
  "vercel/action",
  // Netlify
  "netlify/actions/deploy",
  "nwtgck/actions-netlify",
  // Cloudflare
  "cloudflare/wrangler-action",
  "cloudflare/pages-action",
  // Railway
  "railwayapp/railway-action",
  // Fly.io
  "superfly/flyctl-actions",
  // Render
  "render-oss/render-deploy-action",
  // DigitalOcean
  "digitalocean/action-doctl",

  // === Static Hosting ===
  "jamesives/github-pages-deploy-action",
  "peaceiris/actions-gh-pages",
  "firebase/firebase-tools",
  "FirebaseExtended/action-hosting-deploy",
  "w9jds/firebase-action",

  // === Kubernetes ===
  "azure/k8s-set-context",
  "azure/k8s-create-secret",
  "helm/chart-releaser-action",
  "deliverybot/helm",
  "koslib/helm-eks-action",

  // === Infrastructure as Code ===
  "hashicorp/setup-terraform",
  "pulumi/actions",
  "aws-actions/aws-cdk",

  // === Serverless ===
  "serverless/github-action",
  "aws-actions/aws-lambda-action",

  // === GitHub Releases ===
  "softprops/action-gh-release",
  "ncipollo/release-action",
  "marvinpinto/action-automatic-releases",
];

/**
 * Shell commands that perform publishing or deployment.
 * Jobs with `run:` steps containing these should NOT get `if: always()`.
 */
const sensitiveCommands = [
  // === Package Managers ===
  // JavaScript/Node.js
  "npm publish",
  "yarn publish",
  "pnpm publish",
  "npm dist-tag",
  "yarn npm publish",
  "npx semantic-release",
  "npx changeset publish",
  // Python
  "twine upload",
  "python -m twine",
  "python3 -m twine",
  "poetry publish",
  "flit publish",
  "pdm publish",
  "pip upload",
  // Ruby
  "gem push",
  "gem release",
  "rake release",
  "bundle exec rake release",
  // Rust
  "cargo publish",
  // Go
  "goreleaser release",
  "goreleaser build --snapshot=false",
  // .NET
  "dotnet nuget push",
  "nuget push",
  "dotnet pack && dotnet nuget",
  // Java/Kotlin
  "mvn deploy",
  "mvn release:perform",
  "gradle publish",
  "gradle publishToMaven",
  "./gradlew publish",
  "./mvnw deploy",
  // PHP
  "composer publish",
  // Elixir
  "mix hex.publish",
  // Dart/Flutter
  "dart pub publish",
  "flutter pub publish",
  // Swift/Cocoapods
  "pod trunk push",
  "pod lib lint && pod trunk",

  // === Container Registries ===
  "docker push",
  "docker buildx push",
  "docker-compose push",
  "docker compose push",
  "podman push",
  "buildah push",
  "crane push",
  "skopeo copy",
  // AWS ECR
  "aws ecr get-login",
  "docker login -u AWS",
  // GCR
  "docker push gcr.io",
  "docker push us.gcr.io",
  "docker push eu.gcr.io",
  "docker push asia.gcr.io",
  // Azure ACR
  "az acr login",
  "docker push .azurecr.io",
  // GHCR
  "docker push ghcr.io",

  // === Git Operations ===
  "git push --tags",
  "git push origin refs/tags",
  "git push origin --tags",
  "git tag -a && git push",
  "git push --follow-tags",

  // === GitHub CLI ===
  "gh release create",
  "gh release upload",
  "gh release edit",
  "gh pr merge --auto",

  // === Kubernetes ===
  "kubectl apply",
  "kubectl create",
  "kubectl replace",
  "kubectl set image",
  "kubectl rollout",
  "kubectl patch",
  "kubectl scale",
  "kubectl delete",
  "kubectl drain",
  "kubectl apply -k",
  "kustomize build | kubectl apply",

  // === Helm ===
  "helm install",
  "helm upgrade",
  "helm push",
  "helm package && helm push",
  "helm delete",
  "helm uninstall",
  "helm rollback",

  // === Terraform ===
  "terraform apply",
  "terraform destroy",
  "terraform import",
  "tofu apply",
  "tofu destroy",
  "terragrunt apply",
  "terragrunt destroy",
  "terragrunt run-all apply",

  // === Pulumi ===
  "pulumi up",
  "pulumi update",
  "pulumi destroy",
  "pulumi preview --diff",

  // === AWS CDK ===
  "cdk deploy",
  "cdk destroy",
  "npx cdk deploy",
  "npx aws-cdk deploy",

  // === Cloud CLIs ===
  // AWS
  "aws s3 sync",
  "aws s3 cp",
  "aws s3 mv",
  "aws s3 rm",
  "aws s3api put-object",
  "aws lambda update-function",
  "aws lambda publish",
  "aws ecs update-service",
  "aws ecs deploy",
  "aws cloudformation deploy",
  "aws cloudformation create-stack",
  "aws cloudformation update-stack",
  "aws elasticbeanstalk update-environment",
  "aws amplify start-deployment",
  "sam deploy",
  "sam package && sam deploy",
  // GCP
  "gcloud app deploy",
  "gcloud run deploy",
  "gcloud functions deploy",
  "gcloud compute deploy",
  "gcloud builds submit",
  "gcloud container clusters",
  // Azure
  "az webapp deploy",
  "az functionapp deploy",
  "az acr build",
  "az aks update",
  "az container create",
  "az container app up",

  // === Platform-as-a-Service ===
  // Heroku
  "heroku deploy",
  "heroku releases:create",
  "heroku container:release",
  "heroku container:push",
  "git push heroku",
  // Fly.io
  "flyctl deploy",
  "fly deploy",
  "fly launch",
  "flyctl machine run",
  // Railway
  "railway deploy",
  "railway up",
  // Render
  "render deploy",
  // Vercel
  "vercel --prod",
  "vercel deploy --prod",
  "vercel --production",
  "vercel deploy --production",
  // Netlify
  "netlify deploy --prod",
  "netlify deploy --production",
  // Cloudflare
  "wrangler publish",
  "wrangler deploy",
  "npx wrangler publish",
  "npx wrangler deploy",
  // DigitalOcean
  "doctl apps create-deployment",
  "doctl kubernetes cluster",
  // Dokku
  "dokku deploy",
  "git push dokku",
  // Platform.sh
  "platform deploy",
  "platform push",
  // Aptible
  "aptible deploy",

  // === Serverless ===
  "serverless deploy",
  "sls deploy",
  "npx serverless deploy",
  "npx sls deploy",
  "firebase deploy",
  "firebase hosting:channel:deploy",
  "amplify publish",
  "amplify push",

  // === Database Migrations ===
  "flyway migrate",
  "flyway repair",
  "liquibase update",
  "liquibase rollback",
  "alembic upgrade",
  "alembic downgrade",
  "knex migrate:latest",
  "knex migrate:rollback",
  "prisma migrate deploy",
  "prisma db push",
  "prisma migrate reset",
  "django-admin migrate",
  "python manage.py migrate",
  "rails db:migrate",
  "rake db:migrate",
  "bundle exec rails db:migrate",
  "sequelize db:migrate",
  "typeorm migration:run",
  "goose up",
  "goose down",
  "dbmate up",
  "dbmate down",
  "atlas migrate apply",
  "atlas schema apply",

  // === SSH/Remote Deployment ===
  "ssh .* && ",
  "rsync -avz",
  "scp ",
  "ansible-playbook",
  "fabric deploy",
  "fab deploy",
  "capistrano deploy",
  "cap deploy",
];

/**
 * Pre-built sets for O(1) pattern matching.
 */
const sensitiveJobNamesSet = new Set(sensitiveJobNames);
const sensitiveActionsSet = new Set(sensitiveActions);
const sensitiveCommandsSet = new Set(sensitiveCommands);

/**
 * Checks if a string contains any pattern from the set as a substring.
 */
const _containsSensitiveSubstring = (
  haystack: string,
  patterns: Set<string>
): boolean => {
  for (const pattern of patterns) {
    if (haystack.includes(pattern)) {
      return true;
    }
  }
  return false;
};

/**
 * Reason why a job is considered sensitive.
 */
export interface SensitivityReason {
  readonly type: "job-name" | "action" | "command" | "action-pattern";
  readonly pattern: string;
}

/**
 * Returns true if the job might publish, release, or deploy.
 * These jobs should NOT get `if: always()` to prevent accidental production releases.
 *
 * @param jobId - The job ID from the workflow
 * @param job - The job object
 * @returns Whether the job is considered sensitive
 */
export const isSensitiveJob = (jobId: string, job: Job): boolean => {
  return getSensitivityReason(jobId, job) !== null;
};

/**
 * Returns the reason why a job is sensitive, or null if it's not.
 * Useful for displaying why a job was marked as sensitive.
 *
 * @param jobId - The job ID from the workflow
 * @param job - The job object
 * @returns The reason for sensitivity, or null
 */
export const getSensitivityReason = (
  jobId: string,
  job: Job
): SensitivityReason | null => {
  // Check job ID and name for sensitive keywords
  const nameLower = (job.name || jobId).toLowerCase();

  for (const pattern of sensitiveJobNamesSet) {
    if (nameLower.includes(pattern)) {
      return { type: "job-name", pattern };
    }
  }

  // Check steps for sensitive actions or commands
  for (const step of job.steps ?? []) {
    // Check for publishing/deployment actions
    if (step.uses) {
      const actionLower = step.uses.toLowerCase();

      // Check known dangerous actions
      for (const pattern of sensitiveActionsSet) {
        if (actionLower.includes(pattern.toLowerCase())) {
          return { type: "action", pattern };
        }
      }

      // Check generic patterns in action names
      const genericPatterns = [
        "/deploy",
        "/publish",
        "/release",
        "-deploy",
        "-publish",
        "-release",
      ];
      for (const pattern of genericPatterns) {
        if (actionLower.includes(pattern)) {
          return { type: "action-pattern", pattern };
        }
      }
    }

    // Check run commands for publishing/deployment
    if (step.run) {
      const cmdLower = step.run.toLowerCase();

      for (const pattern of sensitiveCommandsSet) {
        if (cmdLower.includes(pattern.toLowerCase())) {
          return { type: "command", pattern };
        }
      }
    }
  }

  return null;
};

/**
 * Workflow filename patterns that indicate the entire workflow is sensitive.
 */
const sensitiveWorkflowNames = [
  "release",
  "deploy",
  "publish",
  "production",
  "prod",
];

/**
 * Checks if a workflow filename indicates it's a sensitive workflow.
 *
 * @param filename - The workflow filename (e.g., "release.yml")
 * @returns Whether the workflow is sensitive based on its name
 */
export const isSensitiveWorkflow = (filename: string): boolean => {
  const nameLower = filename.toLowerCase();
  return sensitiveWorkflowNames.some((pattern) => nameLower.includes(pattern));
};

/**
 * Formats a sensitivity reason for display.
 *
 * @param reason - The sensitivity reason
 * @returns A human-readable description
 */
export const formatSensitivityReason = (reason: SensitivityReason): string => {
  switch (reason.type) {
    case "job-name":
      return `job name contains "${reason.pattern}"`;
    case "action":
      return `uses ${reason.pattern}`;
    case "action-pattern":
      return `action matches ${reason.pattern}`;
    case "command":
      return `runs "${reason.pattern}"`;
    default: {
      const exhaustiveCheck: never = reason.type;
      return `unknown reason: ${exhaustiveCheck}`;
    }
  }
};
