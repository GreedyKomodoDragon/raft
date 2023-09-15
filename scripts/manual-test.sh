#!/bin/bash

set -e # Exit immediately if a command fails

skip_rm=false

while getopts "k" opt; do
    case $opt in
        k)
            skip_rm=true
            ;;
        *)
            echo "Usage: $0 [-k]"
            exit 1
            ;;
    esac
done

if ! $skip_rm; then
    echo "Removing files..."
    rm -rf testing/node_*
fi

echo "Stopping Docker Compose..."
docker-compose -f testing/docker-compose.yml down

echo "Building Docker image..."
docker build -t raft-test:latest -f testing/raft.dockerfile . 

echo "Cleaning up unused Docker images..."
docker image prune -f

echo "Starting Docker Compose..."
docker-compose -f testing/docker-compose.yml up
