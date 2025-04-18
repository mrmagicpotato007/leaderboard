#!/bin/bash
set -e

echo "Building users_service..."
cd users_service && go mod tidy && go build -o users_service && cd ..

echo "Building score_service..."
cd score_service && go mod tidy && go build -o score_service && cd ..

echo "Building ranking_service..."
cd ranking_service && go mod tidy && go build -o ranking_service && cd ..

echo "Building worker_service..."
cd worker_service && go mod tidy && go build -o worker_service && cd ..

echo "All services built successfully."
