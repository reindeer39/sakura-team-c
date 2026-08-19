#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  echo "Usage: $0 <docker-image>"
  exit 1
fi

IMAGE="$1"
DEPLOY_DIR="/opt/sakuravel"
COMPOSE_FILE="${DEPLOY_DIR}/compose.reg.yml"
ENV_FILE="/opt/sakuravel/backend.env"


if [ ! -f "${COMPOSE_FILE}" ]; then
  echo "${COMPOSE_FILE} does not exist"
  exit 1
fi

export BACKEND_IMAGE="${IMAGE}"

cd "${DEPLOY_DIR}"

if docker container inspect sakuravel-backend >/dev/null 2>&1; then
  echo "Removing legacy container: sakuravel-backend"
  docker rm -f sakuravel-backend
fi

echo "Pulling deployment images"
docker compose -f "${COMPOSE_FILE}" pull

echo "Starting deployment"
docker compose -f "${COMPOSE_FILE}" up -d --remove-orphans

sleep 5

for service in api frontend caddy; do
  if [ -z "$(docker compose -f "${COMPOSE_FILE}" ps --status running -q "${service}")" ]; then
    echo "${service} failed to start"
    docker compose -f "${COMPOSE_FILE}" logs --tail=100 "${service}" || true
    exit 1
  fi
done

echo "Deployment completed: ${IMAGE}"
docker compose --env-file "${ENV_FILE}" -f "${COMPOSE_FILE}" ps
