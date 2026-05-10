FROM golang:1.26-alpine AS builder

WORKDIR /app

COPY go.mod ./
COPY main.go ./

RUN CGO_ENABLED=0 go build -o /mockwebhook .

FROM alpine:3.19

RUN apk add --no-cache ca-certificates

COPY --from=builder /mockwebhook /mockwebhook

EXPOSE 8888

CMD ["/mockwebhook"]