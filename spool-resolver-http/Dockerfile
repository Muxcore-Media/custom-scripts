FROM golang:1.26-alpine AS builder
COPY core/ /build/core/
COPY spool-resolver-http/ /build/spool-resolver-http/
WORKDIR /build/spool-resolver-http
RUN go mod download
RUN CGO_ENABLED=0 go build -o /spool-resolver-http ./cmd/module
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /spool-resolver-http /
ENTRYPOINT ["/spool-resolver-http"]
