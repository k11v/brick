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

# Download fonts.
RUN tlmgr install cm-unicode \
 && mkdir -p /usr/share/fonts/opentype/freefont \
 && curl -sSL "https://ftp.gnu.org/gnu/freefont/freefont-otf-20120503.tar.gz" | tar -vxz -C /usr/share/fonts/opentype/freefont --strip-components 1 --wildcards "*.otf" \
 && mkdir -p /usr/share/fonts/truetype/noto-emoji \
 && curl -sSL -o /usr/share/fonts/truetype/noto-emoji/NotoColorEmoji.ttf "https://github.com/googlefonts/noto-emoji/raw/refs/tags/v2.047/fonts/NotoColorEmoji.ttf"

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
COPY --from=builder /root/bin/build /user/bin/build

# Run application.
USER user:user
VOLUME /user/run
WORKDIR /user/run
ENTRYPOINT ["build"]
