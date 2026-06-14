FROM golang:1.26-alpine AS builder
COPY core/ /build/core/
COPY input-validate-jsonschema/ /build/input-validate-jsonschema/
WORKDIR /build/input-validate-jsonschema
RUN go mod download
RUN CGO_ENABLED=0 go build -o /input-validate-jsonschema ./cmd/module
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /input-validate-jsonschema /
ENTRYPOINT ["/input-validate-jsonschema"]
