# used for testing only

# Build stage
FROM golang:alpine AS builder

RUN apk add --no-cache git && mkdir /app
ADD . /app
WORKDIR /app

# Fetch dependencies
RUN go mod tidy && go build --ldflags '-w -s' -o main cmd/main.go

# Final stage
FROM alpine:latest

RUN apk --no-cache add bash curl ca-certificates && \
    apk update

COPY --from=builder /app/main /app/main

RUN addgroup -S appgroup && \
    adduser -S appuser -G appgroup && \
    chown appuser:appgroup /app/main && \
    mkdir /node_data && \
    chown appuser:appgroup /node_data && \
    chmod 750 /node_data && \
    mkdir /cert && \
    chown appuser:appgroup /cert && \
    chmod 750 /cert

USER appuser

CMD ["/app/main"]