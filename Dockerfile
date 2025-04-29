# 1) Stage: build
FROM golang:1.23.4 AS builder
WORKDIR /app

# Зависимости
COPY go.mod go.sum ./
RUN go mod download

RUN go install github.com/golang-migrate/migrate/v4/cmd/migrate@v4.15.2

# Сборка приложения
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o mapapp .

# 2) Stage: final
FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Копируем бинарь и статические файлы
COPY --from=builder /app/mapapp .
COPY --from=builder /app/static ./static
COPY --from=builder /go/bin/migrate /usr/local/bin/migrate
COPY --from=builder /app/db/migrations ./db/migrations

EXPOSE 8000
CMD ["./mapapp"]
