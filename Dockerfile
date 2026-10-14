# Single Render web service: Go Mujeeb API+worker and private LiteLLM gateway.
# Do not expose the LiteLLM admin port: Render serves the Go API on $PORT.
FROM golang:1.22.2-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/mujeeb-api ./cmd/api \
    && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/mujeeb-worker ./cmd/worker \
    && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/mujeeb-migrate ./cmd/migrate \
    && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/mujeeb-runtime ./cmd/runtime \
    && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/mujeeb-combined ./cmd/combined

# Includes Python, LiteLLM, dashboard, and Prisma DB migration tools.
# Keep release pinned and verify official image security before each upgrade.
FROM ghcr.io/berriai/litellm:v1.103.0

COPY --from=build /out/mujeeb-api /usr/local/bin/mujeeb-api
COPY --from=build /out/mujeeb-worker /usr/local/bin/mujeeb-worker
COPY --from=build /out/mujeeb-migrate /usr/local/bin/mujeeb-migrate
COPY --from=build /out/mujeeb-runtime /usr/local/bin/mujeeb-runtime
COPY --from=build /out/mujeeb-combined /usr/local/bin/mujeeb-combined

# Go binds publicly on Render's PORT (3001); LiteLLM only binds to 127.0.0.1:4000.
EXPOSE 3001
ENTRYPOINT ["/usr/local/bin/mujeeb-combined"]
