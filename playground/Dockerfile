FROM golang:1.25 AS builder

WORKDIR /app
COPY server.go .

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server server.go

FROM alpine:latest

WORKDIR /root/
COPY --from=builder /app/server .

CMD ["./server"]
