FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o atlas-intelligence-engine ./cmd/intelligence-engine

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/atlas-intelligence-engine .
EXPOSE 8081
CMD ["./atlas-intelligence-engine"]
