FROM msai-cn-beijing.cr.volces.com/public/library/golang:1.26.1-bookworm AS base

ARG GOPROXY="https://goproxy.cn,direct"
ARG GONOSUMDB="rpkg.cc,*.msh.team"
RUN go env -w \
  GOPATH="/go" \
  GOCACHE="/go/cache" \
  GOPROXY=$GOPROXY \
  GONOSUMDB=$GONOSUMDB \
  GOEXPERIMENT="jsonv2"

WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go \
  go mod download -x

RUN \
  --mount=type=cache,target=/go \
  --mount=type=bind,target=. \
  go build -o /dist/kimiko .


FROM msai-cn-beijing.cr.volces.com/public/library/debian:bookworm-slim
ARG APT_SOURCE=mirrors.volces.com

WORKDIR /usr/src/app

RUN set -ex \
  && sed -i "s|deb.debian.org|$APT_SOURCE|g" /etc/apt/sources.list.d/debian.sources

RUN \
  --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
  --mount=type=cache,target=/var/cache/apt,sharing=locked \
  apt-get update && apt-get install -y --no-install-recommends \
  ca-certificates tzdata curl

COPY --from=base /dist/kimiko .
CMD ["./kimiko"]
