# Stage 1: Build
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache make git gcc musl-dev linux-headers

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o /usr/local/bin/energychaind ./cmd/energychaind

# Stage 2: Runtime
FROM alpine:3.21

RUN apk add --no-cache ca-certificates jq bash curl

COPY --from=builder /usr/local/bin/energychaind /usr/local/bin/energychaind

EXPOSE 26656 26657 1317 8545 8546 9090

ENTRYPOINT ["energychaind"]
CMD ["start"]
