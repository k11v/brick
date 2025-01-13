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


FROM pandoc/latex:3.6-alpine AS runner

# Install fonts.
RUN tlmgr install cm-unicode \
 && mkdir -p /usr/share/fonts/opentype/freefont \
 && curl -sSL "https://ftp.gnu.org/gnu/freefont/freefont-otf-20120503.tar.gz" | tar -vxz -C /usr/share/fonts/opentype/freefont --strip-components 1 --wildcards "*.otf" \
 && mkdir -p /usr/share/fonts/truetype/noto-emoji \
 && curl -sSL -o /usr/share/fonts/truetype/noto-emoji/NotoColorEmoji.ttf "https://github.com/googlefonts/noto-emoji/raw/refs/tags/v2.047/fonts/NotoColorEmoji.ttf"

# Install app.
ENV PATH="/opt/app/bin:$PATH"
RUN mkdir /opt/app \
 && mkdir /opt/app/bin
COPY --from=builder /opt/app/bin/build /opt/app/bin/

# Prepare environment.
ENV UID=2000 GID=2000 HOME=/user
ENV PATH="$HOME/bin:$PATH"
RUN addgroup -g "$GID" -S user \
 && adduser -u "$UID" -G user -h "$HOME" -H -s /bin/sh -S user \
 && mkdir "$HOME" \
 && mkdir "$HOME/bin" \
 && mkdir "$HOME/run" \
 && chown -R "$UID:$GID" "$HOME"

# Run application.
USER user:user
VOLUME /user/run
WORKDIR /user/run
ENTRYPOINT ["build"]
