# ATLAS Architecture (M0)

## Overview
This document outlines the architecture for the M0 Engineering Foundation of ATLAS. 
The M0 scope is intentionally limited to the foundation layer.

## Services
1. **Control Plane**
   - Technology: Java 25, Spring Boot 4.1.x (or 3.4.x fallback)
   - Role: The central management service for ATLAS (future). Currently provides a basic health check and structured logging.
2. **Intelligence Engine**
   - Technology: Go 1.26
   - Role: Core intelligence platform for root-cause analysis (future). Currently provides a basic health check and structured logging.
3. **ATLAS Agent**
   - Technology: Go 1.26
   - Role: Lightweight agent deployed alongside target services (future). Currently provides a basic health check and structured logging.

## Communication
In M0, services do not communicate with each other. They each expose an HTTP server with a health endpoint.

## Data Storage & Eventing
Currently, no data stores or event buses are implemented. Placeholders for PostgreSQL, Redis, and Kafka exist in the configuration templates for future milestones.

## Infrastructure
- **Containerization**: All services are containerized using Docker multi-stage builds.
- **Orchestration**: Docker Compose is used for local development and testing. Kubernetes deployment is out of scope for M0.
