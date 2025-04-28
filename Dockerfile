FROM rust:1.85-slim as rust-build

WORKDIR /app

RUN apt-get update && apt-get install -y \
    build-essential \
    clang-tools-14 \
    git \
    libssl-dev \
    pkg-config \
    protobuf-compiler \
    libudev-dev \
    && apt-get clean

COPY namada/parse ./

RUN cargo build --release

FROM golang:1.24 AS go-build

WORKDIR /app

COPY --from=rust-build /app/target/release namada/parse/target/release

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 CGO_LDFLAGS=-lm go build

FROM debian:bookworm-slim

COPY --from=go-build /app/migrate-masp-events /app/migrate-masp-events

WORKDIR /app

ENTRYPOINT ["/app/migrate-masp-events"]