FROM golang:1.23-alpine3.20 AS builder

USER root:root
WORKDIR /root/src/

# Install dependencies.
COPY ./go.mod ./go.sum ./
RUN go mod download

# Copy sources.
COPY ./cmd/ ./cmd/
COPY ./internal/ ./internal/

# Build application.
RUN --mount=type=cache,target=/root/gocache \
    GOCACHE=/root/gocache CGO_ENABLED=0 \
    go build -o /root/bin/ ./cmd/...


FROM pandoc/latex:3.6-alpine AS runner

# Prepare environment.
ENV UID=2000 GID=2000 HOME=/user
ENV PATH="$HOME/bin:$PATH"
RUN addgroup -g "$GID" -S user \
 && adduser -u "$UID" -G user -h "$HOME" -H -s /bin/sh -S user \
 && mkdir "$HOME" \
 && mkdir "$HOME/bin" \
 && mkdir "$HOME/run" \
 && chown -R "$UID:$GID" "$HOME"

# Copy application.
COPY --from=builder /root/bin/runner /user/bin/runner

# Run application.
USER user:user
ENTRYPOINT ["runner"]
