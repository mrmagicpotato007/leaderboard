#!/bin/bash

echo "Starting worker service."
cd ./worker_service && ./worker_service -mode cassandra &

echo "Starting worker service."
cd ./worker_service && ./worker_service -mode redis &

echo "Starting ranking service "
cd ./ranking_service && ./ranking_service &

echo "Starting score service"
cd ./score_service && ./score_service &

echo "Starting user service."
cd ./users_service && ./users_service &


# Wait for all background processes to complete
wait

echo "All services are running."