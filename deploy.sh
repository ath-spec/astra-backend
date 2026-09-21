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

echo "==> Downloading AWS RDS SSL Certificate"
curl -sS -o global-bundle.pem https://truststore.pki.rds.amazonaws.com/global/global-bundle.pem

echo "==> Building image: $IMAGE:$TAG"
docker build --platform linux/arm64 -t "$IMAGE:$TAG" .

echo "==> Pushing image"
docker push "$IMAGE:$TAG"

echo "==> Applying Kubernetes configurations (ConfigMaps, Secrets, Services, Deployment)"
kubectl apply -f k8s/

echo "==> Cleaning up any previously stuck pods (ImagePullBackOff/CrashLoopBackOff)"
# This forces Kubernetes to delete stuck pods and rely on the new ReplicaSet
kubectl delete pods -l app=astra-backend --field-selector=status.phase!=Running 2>/dev/null || true

echo "==> Updating Kubernetes deployment with new image tag"
kubectl set image deployment/astra-backend \
  astra-backend="$IMAGE:$TAG"

echo "==> Waiting for rollout"
# Use a timeout so it doesn't hang forever if the new code crashes
kubectl rollout status deployment/astra-backend --timeout=120s

echo "==> Deployment successful"
echo "Image: $IMAGE:$TAG"
