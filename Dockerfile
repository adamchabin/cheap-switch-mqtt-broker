# Build stage
FROM golang:1.25-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /poe-mqtt-bridge ./cmd/broker

# Run stage
FROM alpine:latest
WORKDIR /app
COPY --from=build /poe-mqtt-bridge /poe-mqtt-bridge
ENTRYPOINT ["/poe-mqtt-bridge"]
