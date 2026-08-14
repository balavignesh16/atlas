.PHONY: build test lint run docker-build docker-up docker-down

build:
	@echo "Building Control Plane..."
	cd services/control-plane && mvn clean package -DskipTests
	@echo "Building Intelligence Engine..."
	cd services/intelligence-engine && go build -o atlas-intelligence-engine .
	@echo "Building ATLAS Agent..."
	cd agents/atlas-agent && go build -o atlas-agent .
	@echo "Building Gateway..."
	cd services/gateway && mvn clean package -DskipTests
	@echo "Building Order Service..."
	cd services/order-service && mvn clean package -DskipTests

test:
	@echo "Testing Control Plane..."
	cd services/control-plane && mvn test
	@echo "Testing Intelligence Engine..."
	cd services/intelligence-engine && go test -v ./...
	@echo "Testing ATLAS Agent..."
	cd agents/atlas-agent && go test -v ./...
	@echo "Testing Gateway..."
	cd services/gateway && mvn test
	@echo "Testing Order Service..."
	cd services/order-service && mvn test

lint:
	@echo "Formatting Go code..."
	cd services/intelligence-engine && go fmt ./...
	cd agents/atlas-agent && go fmt ./...
	@echo "Vetting Go code..."
	cd services/intelligence-engine && go vet ./...
	cd agents/atlas-agent && go vet ./...

run:
	@echo "Run command is not fully defined yet, please use docker-up instead for local development."

docker-build:
	@echo "Building Linux binaries for Docker..."
	cd services/intelligence-engine && $env:GOOS="linux"; $env:CGO_ENABLED="0"; go build -o atlas-intelligence-engine .
	cd agents/atlas-agent && $env:GOOS="linux"; $env:CGO_ENABLED="0"; go build -o atlas-agent .
	cd services/control-plane && mvn clean package -DskipTests
	cd services/gateway && mvn clean package -DskipTests
	cd services/order-service && mvn clean package -DskipTests
	docker-compose build

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down
