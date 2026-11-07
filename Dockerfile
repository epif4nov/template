# Stage 1: Build
FROM golang:1.27-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /trip-service \
    ./cmd/trip-service

# Stage 2: Runtime
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /

COPY --from=builder /trip-service /trip-service

USER 65532:65532

EXPOSE 8080

ENTRYPOINT ["/trip-service"]