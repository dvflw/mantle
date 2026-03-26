# Stage 1: Build the mantle binary
FROM golang:1.25-alpine AS builder

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

RUN apk add --no-cache git

WORKDIR /src

# Cache dependency downloads
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w \
      -X github.com/dvflw/mantle/internal/version.Version=${VERSION} \
      -X github.com/dvflw/mantle/internal/version.Commit=${COMMIT} \
      -X github.com/dvflw/mantle/internal/version.Date=${DATE}" \
    -o /mantle ./cmd/mantle

# Stage 2: Minimal runtime image
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S mantle \
    && adduser -S -G mantle -h /home/mantle -s /bin/sh mantle

COPY --from=builder /mantle /usr/local/bin/mantle

ENV MANTLE_LOG_LEVEL=info
ENV MANTLE_API_ADDRESS=:8080

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/healthz || exit 1

USER mantle
WORKDIR /home/mantle

ENTRYPOINT ["mantle"]
