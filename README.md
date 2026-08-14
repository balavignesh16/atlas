# ATLAS — Backend Intelligence & Reliability Platform

## Overview
ATLAS is a distributed backend intelligence and reliability platform. It is designed to observe backend services, understand service dependencies, detect incidents, investigate probable root causes, recommend safe remediation, execute approved remediation workflows, and verify system recovery.

## Current Scope (M0 Engineering Foundation)
This repository currently contains the **M0 Engineering Foundation**. 
It strictly establishes the monorepo structure, Go and Spring Boot foundations, basic CI, Docker setup, configuration, logging, health-check, and testing conventions. 

**Intentionally NOT implemented yet in M0:**
- Kafka event processing
- Telemetry collection & OpenTelemetry pipelines
- PostgreSQL schema and Redis integration
- Dependency graph & Incident engine
- Root-cause analysis & AI reasoning
- Remediation engine & Chaos engineering
- Dashboard & Authentication system
- Production Kubernetes deployment

## Repository Structure
```
atlas/
├── services/
│   ├── control-plane/        (Java Spring Boot application)
│   └── intelligence-engine/  (Go application)
├── agents/
│   └── atlas-agent/          (Go application)
├── infrastructure/
│   ├── docker/               (Docker configuration)
│   └── scripts/              (Helper scripts)
├── configs/                  (Configuration templates)
├── docs/                     (Documentation)
├── tests/                    (E2E and Integration tests)
├── .github/workflows/        (CI pipelines)
├── .env.example              (Environment variables template)
├── Makefile                  (Development tasks)
└── docker-compose.yml        (Local environment setup)
```

## Technologies Used
Current local development versions:
- **Java 24**
- **Spring Boot 3.4.2**
- **Go 1.24**
- **Docker**
- **Maven**

*Note:* These are temporary compatibility fallbacks used because of the current local development environment and Docker/toolchain download issues. 
- **Java 25** remains the intended modern Java baseline for future upgrade when the local environment permits.
- **Go 1.26** remains the intended modern Go baseline for future upgrade when the environment permits.
- **Reproducible source-based Docker builds** are explicitly marked as a future engineering improvement. Currently, the Docker build for Go uses host-compiled Linux binaries as a temporary development workaround to avoid network timeouts during toolchain downloads inside containers.

### Reserved for Future Modules
- PostgreSQL
- Redis
- Kafka
- OpenTelemetry
- React

## Prerequisites
- Docker & Docker Compose
- Java 25 & Maven
- Go 1.26
- Make

## How to Run Locally
1. Copy `.env.example` to `.env`.
2. Run `make docker-build` to build the Docker images.
3. Run `make docker-up` to start the services in the background.
4. Run `make docker-down` to stop them.

## Current Health Endpoints
- Control Plane: `http://localhost:8080/actuator/health`
- Intelligence Engine: `http://localhost:8081/health`
- ATLAS Agent: `http://localhost:8082/health`

## How to Run Tests
- Run `make test` to execute all unit/module tests.
- Run `make lint` to format and vet the Go code.
