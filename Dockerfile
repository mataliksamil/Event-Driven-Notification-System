FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -o /server ./cmd/server
RUN CGO_ENABLED=0 go build -o /worker ./cmd/worker

FROM alpine:3.19

RUN apk add --no-cache ca-certificates

COPY --from=builder /server /server
COPY --from=builder /worker /worker
COPY --from=builder /app/migrations /migrations
COPY --from=builder /app/oapi.yaml /oapi.yaml

EXPOSE 8080 9090

CMD ["/server"]