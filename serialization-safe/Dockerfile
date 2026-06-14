FROM golang:1.26-alpine AS builder
COPY core/ /build/core/
COPY serialization-safe/ /build/serialization-safe/
WORKDIR /build/serialization-safe
RUN go mod download
RUN CGO_ENABLED=0 go build -o /serialization-safe ./cmd/module
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /serialization-safe /
ENTRYPOINT ["/serialization-safe"]
