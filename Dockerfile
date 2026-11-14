# syntax=docker/dockerfile:1
# golang:1.25-alpine
FROM --platform=$BUILDPLATFORM golang@sha256:1ae0735f00daffa3aaf1363a5184c0d2dc55c78e3db4ec70241cdac97bf84b59 AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X main.version=$VERSION" -o /out/kenshoo ./cmd/kenshoo

# alpine:3.21
FROM alpine@sha256:bc21487101ae47172509130ee1092cc273a831968d64b617776d1f69c81e42ab
RUN apk add --no-cache ca-certificates postgresql16-client \
 && pg_restore --version | grep -q "(PostgreSQL) 16" \
 && adduser -D -H -u 65532 kenshoo
COPY --from=build /out/kenshoo /usr/local/bin/kenshoo
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/kenshoo"]
CMD ["--config", "/etc/kenshoo/kenshoo.yaml"]
