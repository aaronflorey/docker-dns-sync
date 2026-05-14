FROM golang:1.26.3 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/docker-dns-sync ./cmd/docker-dns-sync

FROM gcr.io/distroless/static-debian12

WORKDIR /
COPY --from=build /out/docker-dns-sync /usr/local/bin/docker-dns-sync

ENTRYPOINT ["/usr/local/bin/docker-dns-sync"]
CMD ["-config", "/etc/docker-dns-sync/config.toml"]
