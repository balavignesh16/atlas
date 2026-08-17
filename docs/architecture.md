# ATLAS Architecture (M2.1)

## Overview
This document outlines the architecture for ATLAS LAB up to Milestone 2.1 (OpenTelemetry Foundation).
The system consists of core ATLAS components and the ATLAS LAB business workload.

## Business Workload (ATLAS LAB)
1. **API Gateway (`atlas-gateway`)**: Spring Boot 3.4.2 API Gateway.
2. **Order Service (`atlas-order-service`)**: Spring Boot 3.4.2 microservice coordinating orders.
3. **Inventory Service (`atlas-inventory-service`)**: Spring Boot 3.4.2 microservice managing stock.
4. **Payment Service (`atlas-payment-service`)**: Spring Boot 3.4.2 microservice handling payments and sandbox failures.

## Core ATLAS Platform
1. **Control Plane**: Central management service (Future).
2. **Intelligence Engine**: Go 1.26 core intelligence platform (Future).
3. **ATLAS Agent**: Go 1.26 lightweight agent deployed alongside target services (Future).

## Observability Foundation
- **OpenTelemetry Collector (`otel-collector`)**: Deployed alongside services to receive OTLP telemetry (traces and metrics).
- **Instrumentation**: Spring Boot services use Micrometer Tracing bridged to OpenTelemetry.
- **Context Propagation**: W3C `traceparent` headers are automatically injected into downstream requests.

## Communication
- Client → Gateway → Order Service
- Order Service → Inventory Service
- Order Service → Payment Service
- All Java Services → OpenTelemetry Collector (via OTLP HTTP/gRPC)

## Infrastructure
- **Containerization**: All services are containerized using Docker multi-stage builds.
- **Orchestration**: Docker Compose is used for local development, testing, and telemetry verification.
