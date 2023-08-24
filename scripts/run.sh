#!/bin/bash

# Check if the .env file path is provided as an argument
if [ $# -eq 0 ]; then
    echo "Usage: $0 <path_to_env_file>"
    exit 1
fi

# Read the .env file and export the variables as environment variables
export $(cat "$1" | xargs)

# Run the Go module using `go run`
go run cmd/main.go
