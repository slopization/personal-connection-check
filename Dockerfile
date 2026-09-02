# syntax=docker/dockerfile:1
FROM node:26.0.0-alpine3.22@sha256:60ff302eeca050b69ab0740f91c66207dc70f5fed1fb3c778c844d769a311443 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --ignore-scripts
COPY web/ ./
RUN npm run build

FROM golang:1.27.1-alpine3.23@sha256:c8500dc1e6c8d8db60a2c6986bc591517c3be360ca448b9df43449dec430cc34 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# This exact generated directory is excluded from the build context; remove only it
# before copying the freshly built frontend so go:embed cannot see stale output.
RUN rm -rf /src/internal/webui/dist && mkdir -p /src/internal/webui
COPY --from=web /src/web/dist /src/internal/webui/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/pcc ./cmd/pcc

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
LABEL org.opencontainers.image.source="https://git.kyu.sh/modelgarden/personal-connection-check" org.opencontainers.image.licenses="MIT"
COPY --from=build /out/pcc /pcc
USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD ["/pcc", "healthcheck"]
ENTRYPOINT ["/pcc"]
