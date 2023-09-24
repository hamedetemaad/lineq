FROM golang:1.20 AS builder
WORKDIR /app

COPY . .
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o service .

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/service .
CMD ["./service"]