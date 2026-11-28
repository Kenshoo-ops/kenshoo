# syntax=docker/dockerfile:1
# golang:1.26-alpine
FROM --platform=$BUILDPLATFORM golang@sha256:30b1e546a9c7fa3b747c2bdba3a0c7bdf3a02ebb4c5b8405d2f948d7e8aac4ec AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o /out/kenshoo ./cmd/kenshoo

# alpine:3.21
FROM alpine@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf
RUN apk add --no-cache ca-certificates postgresql16-client \
 && pg_restore --version | grep -q "(PostgreSQL) 16" \
 && adduser -D -H -u 65532 kenshoo
COPY --from=build /out/kenshoo /usr/local/bin/kenshoo
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/kenshoo"]
CMD ["--config", "/etc/kenshoo/kenshoo.yaml"]
