FROM golang:1.23-alpine AS builder

USER root:root
WORKDIR /opt/app/src
COPY ./cmd ./cmd
COPY ./internal ./internal
COPY ./go.mod ./go.mod
COPY ./go.sum ./go.sum
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/root/.cache/go-mod \
    export GOCACHE=/root/.cache/go-build \
 && export GOMODCACHE=/root/.cache/go-mod \
 && export CGO_ENABLED=0 \
 && go build -o /opt/app/bin/ ./cmd/...


FROM alpine AS runner

USER root:root
RUN addgroup -g 2000 -S user \
 && adduser -u 2000 -G user -h /user -H -s /bin/sh -S user \
 && mkdir /user \
 && chown 2000:2000 /user

RUN apk add --no-cache curl

ENV PATH="/opt/app/bin:$PATH"
RUN mkdir /opt/app
COPY --from=builder /opt/app/bin /opt/app/bin

USER user:user
WORKDIR /user
ENTRYPOINT []
