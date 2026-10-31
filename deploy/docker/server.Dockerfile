FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
ARG VERSION=0.4.2
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /safe-devops ./cmd/safe-devops

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/* \
    && groupadd -g 10001 safeops && useradd -u 10001 -g safeops -M safeops
COPY --from=build /safe-devops /usr/local/bin/safe-devops
RUN ln -s /usr/local/bin/safe-devops /usr/local/bin/safeops
USER 10001:10001
ENTRYPOINT ["/usr/local/bin/safe-devops"]
CMD ["version"]
