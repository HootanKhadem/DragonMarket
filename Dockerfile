# syntax=docker/dockerfile:1

# ---- Build stage ----
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Cache dependency downloads separately from source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server

# ---- Runtime stage ----
FROM alpine:3.22 AS runtime

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /out/server ./server

EXPOSE 8080

ENTRYPOINT ["./server"]
