FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/app

FROM alpine:3.23

WORKDIR /app

RUN apk add --no-cache tzdata ca-certificates

COPY --from=builder /app/server .

EXPOSE 8080

CMD ["./server"]