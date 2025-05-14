FROM golang:1.24.1 AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . ./

RUN go build -ldflags "-w -s" -a -o main

ENTRYPOINT ["/app/main"]