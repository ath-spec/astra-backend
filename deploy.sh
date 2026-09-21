#!/bin/bash

set -e

REPO_DIR="/usr/bin/zeyro/astra-backend"
# Use the ECR URL provided by the user
IMAGE="070443470895.dkr.ecr.ap-south-1.amazonaws.com/abhimanyu-gupta-ecr"

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

echo "==> Authenticating with AWS ECR"
aws ecr get-login-password --region ap-south-1 | docker login --username AWS --password-stdin 070443470895.dkr.ecr.ap-south-1.amazonaws.com

echo "==> Downloading AWS RDS SSL Certificate"
curl -sS -o global-bundle.pem https://truststore.pki.rds.amazonaws.com/global/global-bundle.pem

echo "==> Building image: $IMAGE:$TAG"
docker build -t "$IMAGE:$TAG" .

echo "==> Pushing image"
docker push "$IMAGE:$TAG"

echo "==> Applying Kubernetes configurations (ConfigMaps, Secrets, Services, Deployment)"
kubectl apply -f k8s/

echo "==> Updating Kubernetes deployment with new image tag"
kubectl set image deployment/astra-backend \
  astra-backend="$IMAGE:$TAG"

echo "==> Waiting for rollout"
kubectl rollout status deployment/astra-backend

echo "==> Deployment successful"
echo "Image: $IMAGE:$TAG"
