FROM golang:1.26-alpine AS builder
COPY core/ /build/core/
COPY notification-default/ /build/notification-default/
WORKDIR /build/notification-default
RUN go mod download
RUN CGO_ENABLED=0 go build -o /notification-default ./cmd/module
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /notification-default /
ENTRYPOINT ["/notification-default"]
