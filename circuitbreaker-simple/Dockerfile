FROM golang:1.26-alpine AS builder
COPY core/ /build/core/
COPY circuitbreaker-simple/ /build/circuitbreaker-simple/
WORKDIR /build/circuitbreaker-simple
RUN go mod download
RUN CGO_ENABLED=0 go build -o /circuitbreaker-simple ./cmd/module
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /circuitbreaker-simple /
ENTRYPOINT ["/circuitbreaker-simple"]
