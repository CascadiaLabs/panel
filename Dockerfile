FROM --platform=$BUILDPLATFORM node:20-alpine AS web
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.27-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Fresh frontend from the web stage must win over any stale api/static in the repo.
COPY --from=web /app/web/dist ./api/static
ARG TARGETOS TARGETARCH
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w -X main.version=$VERSION" -o /app/panel ./cmd/panel

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=build /app/panel /app/panel
EXPOSE 2083
ENTRYPOINT ["/app/panel"]
