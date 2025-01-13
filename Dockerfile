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


FROM alpine:3.20 AS runner

# Install dependencies.
RUN apk add --no-cache curl

# Prepare environment.
ENV UID=2000 GID=2000 HOME=/user
ENV PATH="$HOME/bin:$PATH"
RUN addgroup -g "$GID" -S user \
 && adduser -u "$UID" -G user -h "$HOME" -H -s /bin/sh -S user \
 && mkdir "$HOME" \
 && mkdir "$HOME/bin" \
 && chown -R "$UID:$GID" "$HOME"

# Copy application.
COPY --from=builder /root/bin/ /user/bin/

# Run application.
EXPOSE 8080
USER user:user
WORKDIR /user/
ENTRYPOINT ["build"]
