FROM golang:1.26-alpine AS builder
COPY core/ /build/core/
COPY data-redaction-pattern/ /build/data-redaction-pattern/
WORKDIR /build/data-redaction-pattern
RUN go mod download
RUN CGO_ENABLED=0 go build -o /data-redaction-pattern ./cmd/module
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /data-redaction-pattern /
ENTRYPOINT ["/data-redaction-pattern"]
