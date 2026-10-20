FROM rust:1.79-slim-bookworm AS build
RUN apt-get update \
    && apt-get install -y --no-install-recommends pkg-config libssl-dev cmake build-essential \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY Cargo.toml ./
COPY src ./src
COPY migrations ./migrations
RUN cargo build --release --bin tracking

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates libssl3 \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /app/target/release/tracking /usr/local/bin/tracking
USER 10001
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/tracking"]
