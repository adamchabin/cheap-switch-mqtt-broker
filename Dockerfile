# Build stage
FROM golang:1.22-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /poe-mqtt-bridge ./cmd/broker

# Run stage
FROM alpine:latest
WORKDIR /app
COPY --from=build /poe-mqtt-bridge /poe-mqtt-bridge
ENV MQTT_BROKER=tcp://mosquitto:1883
ENV MQTT_USERNAME=
ENV MQTT_PASSWORD=
ENV MQTT_TOPIC=poe/#
ENV MQTT_CLIENT_ID=cheap-switch-mqtt-bridge
ENTRYPOINT ["/poe-mqtt-bridge"]
