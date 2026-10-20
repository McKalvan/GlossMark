FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY services/payout-notifier/go.* ./
RUN go mod download
COPY services/payout-notifier/ ./
RUN CGO_ENABLED=1 go build -tags musl -o /out/notifier ./cmd/notifier

FROM gcr.io/distroless/base-debian12:nonroot
COPY --from=build /out/notifier /notifier
COPY services/payout-notifier/config /config
ENTRYPOINT ["/notifier", "-config", "/config/consumer-group.yaml"]
