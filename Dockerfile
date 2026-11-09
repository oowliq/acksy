# syntax=docker/dockerfile:1

# The SPA is platform-independent and Go cross-compiles, so both build stages
# run on the build host; only the final stage is per target platform.
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /src/web
RUN npm install -g pnpm@12.16.1
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN pnpm build

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
      -ldflags "-s -w -X github.com/oowliq/acksy/internal/buildinfo.Version=${VERSION} -X github.com/oowliq/acksy/internal/buildinfo.Commit=${COMMIT} -X github.com/oowliq/acksy/internal/buildinfo.Date=${DATE}" \
      -o /out/acksy ./cmd/acksy \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=server /out/acksy /usr/local/bin/acksy
COPY --from=server --chown=65532:65532 /out/data /data
ENV ACKSY_LISTEN=:8080 \
    ACKSY_DATABASE_URL=sqlite:///data/acksy.db
VOLUME /data
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/usr/local/bin/acksy"]
CMD ["serve"]
