#!/bin/bash
# Creates and pushes a git tag for the current CLI version
# Idempotent: succeeds if tag already exists

set -e

VERSION=$(node -p "require('./apps/cli/package.json').version")
TAG="v$VERSION"

echo "Creating tag $TAG..."

# Create tag if it doesn't exist
if git tag -l | grep -q "^${TAG}$"; then
  echo "Tag $TAG already exists locally"
else
  git tag "$TAG"
  echo "Created tag $TAG"
fi

# Push tag if not already pushed
if git ls-remote --tags origin | grep -q "refs/tags/${TAG}$"; then
  echo "Tag $TAG already exists on remote"
else
  git push origin "$TAG"
  echo "Pushed tag $TAG to origin"
fi

echo "Done: $TAG"
