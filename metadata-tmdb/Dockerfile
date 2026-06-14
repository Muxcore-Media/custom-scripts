FROM golang:1.26-alpine AS builder
COPY core/ /build/core/
COPY metadata-tmdb/ /build/metadata-tmdb/
WORKDIR /build/metadata-tmdb
RUN go mod download
RUN CGO_ENABLED=0 go build -o /metadata-tmdb ./cmd/module
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /metadata-tmdb /
ENTRYPOINT ["/metadata-tmdb"]
