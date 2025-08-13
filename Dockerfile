# syntax=docker/dockerfile:1.6

# -------- builder --------
FROM --platform=$BUILDPLATFORM golang:1.22 as builder

ARG TARGETOS=linux
ARG TARGETARCH
WORKDIR /src

# Cache deps first
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build statically for minimal image
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags="-s -w" -o /out/doks-lb-scale ./


# -------- runtime --------
FROM gcr.io/distroless/base-debian12:nonroot

WORKDIR /
COPY --from=builder /out/doks-lb-scale /doks-lb-scale

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/doks-lb-scale"]


