#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  echo "Usage: $0 <docker-image>"
  exit 1
fi

IMAGE="$1"
CONTAINER_NAME="sakuravel-backend"
ENV_FILE="/opt/sakuravel/backend.env"

if [ ! -f "${ENV_FILE}" ]; then
  echo "${ENV_FILE} does not exist"
  exit 1
fi

echo "Pulling ${IMAGE}"
docker pull "${IMAGE}"

echo "Removing old container"
docker rm -f "${CONTAINER_NAME}" 2>/dev/null || true

echo "Starting ${IMAGE}"
docker run -d \
  --name "${CONTAINER_NAME}" \
  --restart unless-stopped \
  --env-file "${ENV_FILE}" \
  -p 8080:8080 \
  "${IMAGE}"

sleep 3

if ! docker ps --format '{{.Names}}' | grep -qx "${CONTAINER_NAME}"; then
  echo "Container failed to start"
  docker logs "${CONTAINER_NAME}" || true
  exit 1
fi

echo "Deployment completed: ${IMAGE}"
docker ps --filter "name=${CONTAINER_NAME}"
