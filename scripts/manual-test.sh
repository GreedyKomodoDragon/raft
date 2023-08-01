#!/bin/bash

set -e # Exit immediately if a command fails

echo "Stopping Docker Compose..."
docker-compose -f testing/docker-compose.yml down

echo "Building Docker image..."
docker build -t raft-test:latest -f testing/raft.dockerfile . 

echo "Cleaning up unused Docker images..."
docker image prune -f

echo "Starting Docker Compose..."
docker-compose -f testing/docker-compose.yml up