FROM golang:1.24-alpine AS builder
WORKDIR /build
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -o /app/server ./cmd/server

FROM alpine:3.20
WORKDIR /app
COPY --from=builder /app/server /app/server
COPY backend/migrations /app/migrations
RUN mkdir -p /data && apk add --no-cache ca-certificates poppler-utils
EXPOSE 8080
ENTRYPOINT ["/app/server"]
