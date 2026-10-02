FROM golang:1.22.2-bookworm AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/mujeeb-api ./cmd/api \
    && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/mujeeb-worker ./cmd/worker \
    && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/mujeeb-migrate ./cmd/migrate

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/mujeeb-api /usr/local/bin/mujeeb-api
COPY --from=build /out/mujeeb-worker /usr/local/bin/mujeeb-worker
COPY --from=build /out/mujeeb-migrate /usr/local/bin/mujeeb-migrate
COPY --from=build /out/mujeeb-runtime /usr/local/bin/mujeeb-runtime

USER nonroot:nonroot
EXPOSE 3001
CMD ["/usr/local/bin/mujeeb-runtime"]
