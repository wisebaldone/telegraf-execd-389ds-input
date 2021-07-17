# syntax=docker/dockerfile:1
FROM golang:1.16 AS builder
WORKDIR /build
COPY . .
RUN go build -o telegraf-execd-389ds-db-input cmd/main.go

FROM telegraf:latest
COPY --from=builder /build/telegraf-execd-389ds-db-input .

