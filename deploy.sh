#!/bin/bash

set -e

REPO_DIR="/usr/bin/zeyro/astra-backend"
# Use the Docker Hub username provided by the user in the k8s manifests
IMAGE="fieryice24/astra-backend-api"

# We check if the script is running in the correct directory, if not, we try to CD into it
if [ ! -d "$REPO_DIR" ]; then
    echo "Warning: Directory $REPO_DIR does not exist. Using current directory."
    REPO_DIR=$(pwd)
fi

cd "$REPO_DIR"

echo "==> Pulling latest code"
git pull origin main

# Get the short git commit hash for tagging
TAG=$(git rev-parse --short HEAD)

echo "==> Building image: $IMAGE:$TAG"
docker build -t "$IMAGE:$TAG" .

echo "==> Pushing image"
docker push "$IMAGE:$TAG"

echo "==> Updating Kubernetes deployment"
kubectl set image deployment/astra-backend \
  astra-backend="$IMAGE:$TAG"

echo "==> Waiting for rollout"
kubectl rollout status deployment/astra-backend

echo "==> Deployment successful"
echo "Image: $IMAGE:$TAG"
