# syntax=docker/dockerfile:1

# ----- build stage -----
FROM golang:1.23-alpine AS build
WORKDIR /src

# Cache deps
COPY go.mod go.sum ./
RUN go mod download

# Build static binary
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/agnostic-ai \
    ./cmd/agnostic-ai

# ----- runtime stage -----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/agnostic-ai /usr/local/bin/agnostic-ai
WORKDIR /work
ENTRYPOINT ["/usr/local/bin/agnostic-ai"]
CMD ["--help"]
