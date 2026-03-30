FROM golang:1.20-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/aegis-api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/aegis-worker ./cmd/worker

FROM alpine:3.20

RUN apk add --no-cache ca-certificates && \
    addgroup -S aegis && \
    adduser -S aegis -G aegis

WORKDIR /app

COPY --from=builder /out/aegis-api /app/aegis-api
COPY --from=builder /out/aegis-worker /app/aegis-worker
COPY --from=builder /src/migrations /app/migrations

USER aegis

EXPOSE 8080

CMD ["/app/aegis-api"]
