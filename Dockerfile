# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o /out/kenshoo ./cmd/kenshoo

FROM alpine:3.24@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf
RUN apk add --no-cache ca-certificates postgresql16-client \
 && pg_restore --version | grep -q "(PostgreSQL) 16" \
 && adduser -D -H -u 65532 kenshoo
COPY --from=build /out/kenshoo /usr/local/bin/kenshoo
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/kenshoo"]
CMD ["--config", "/etc/kenshoo/kenshoo.yaml"]
