# @gloss deploys gmi:PullRequestApp scope=file
# The hosted pull-request app (gloss app serve, docs/APP.md). The image
# holds the static gloss binary and git, which gloss diff shells out to
# for the base tree and the history section, so the runtime is Alpine
# rather than distroless/static (ADR 0052); it runs as an unprivileged user.
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=0.0.0-dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X github.com/mckalvan/glossmark/internal/version.Version=${VERSION}" -o /gloss ./cmd/gloss

FROM alpine:3.20
RUN apk add --no-cache git ca-certificates && adduser -D -u 10001 gloss && mkdir -p /work && chown gloss /work
COPY --from=build /gloss /usr/local/bin/gloss
USER gloss
ENV LISTEN_ADDR=:8080 WORKDIR=/work
EXPOSE 8080
ENTRYPOINT ["gloss", "app", "serve"]
