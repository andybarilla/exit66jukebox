# syntax=docker/dockerfile:1

FROM node:24-alpine AS web-build
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS go-build
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
COPY --from=web-build /src/internal/web/dist ./internal/web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/exit66jukebox .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates ffmpeg tzdata \
    && addgroup -S exit66 \
    && adduser -S -G exit66 -h /app exit66 \
    && mkdir -p /app /data /music \
    && chown -R exit66:exit66 /app /data
COPY --from=go-build --chown=exit66:exit66 /out/exit66jukebox /usr/local/bin/exit66jukebox
USER exit66:exit66
WORKDIR /data
EXPOSE 8066
VOLUME ["/data", "/music"]
ENTRYPOINT ["exit66jukebox"]
CMD ["-addr", ":8066", "-db", "/data/exit66.db", "-root", "/music"]
