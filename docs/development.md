# Development Guide

## Environment Setup
1. Clone the repository.
2. Copy `.env.example` to `.env`.
3. Ensure Docker, Make, Go 1.26, Java 25, and Maven are installed locally.

## Local Commands
- `make build`: Builds all applications locally.
- `make test`: Runs unit tests for all applications.
- `make lint`: Runs format and vet on Go applications.
- `make docker-build`: Builds Docker images for the complete foundation.
- `make docker-up`: Starts the local Docker Compose environment in detached mode.
- `make docker-down`: Stops the local Docker Compose environment.

## Coding Conventions
### Go
- Current local development version is Go 1.24 (temporary fallback due to toolchain download issues).
- **Go 1.26** remains the intended modern Go baseline for future upgrade when the environment permits.
- Standard formatting applies (`go fmt`).
- Use `log/slog` for structured JSON logging.
- Handle graceful shutdown for SIGINT and SIGTERM.

### Java (Spring Boot)
- Current local development versions are Java 24 and Spring Boot 3.4.2 (temporary fallback due to local compiler compatibility).
- **Java 25 LTS** remains the intended modern Java baseline for future upgrade when the local environment permits.
- Use Maven for dependency management.
- Expose `/actuator/health` for health checks.
- Utilize Spring Boot's built-in graceful shutdown via `server.shutdown=graceful`.

## Branching & Workflow
- Use standard Git feature branching (e.g., `feature/M1-kafka-integration`).
- Ensure all tests and linting pass (`make lint`, `make test`) before creating a Pull Request.
- The CI pipeline will automatically verify formatting, tests, and builds for all PRs.
