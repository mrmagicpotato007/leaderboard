#!/bin/bash

cd "$(dirname "$0")"

# Ensure we're in the monitoring directory
echo "Working directory: $(pwd)"

# Stop any existing containers
echo "Stopping any existing monitoring containers..."
docker-compose down

# Create necessary directories if they don't exist
mkdir -p ./prometheus
mkdir -p ./grafana/provisioning/datasources
mkdir -p ./grafana/provisioning/dashboards

# Check if prometheus.yml exists
if [ ! -f "./prometheus/prometheus.yml" ]; then
  echo "Prometheus config not found, creating it..."
  cp ./prometheus.yml ./prometheus/prometheus.yml 2>/dev/null || echo "Error: prometheus.yml not found"
fi

# Start the monitoring stack
echo "Starting Prometheus and Grafana monitoring stack..."
docker-compose up -d

# Check if containers are running
echo "Checking container status:"
docker-compose ps

echo "\nMonitoring stack started!"
echo "Access Grafana at http://localhost:3000"
echo "Default credentials: admin/admin"
echo "Access Prometheus at http://localhost:9090"
