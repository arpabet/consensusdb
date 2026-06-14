FROM shvid/ubuntu-golang as builder

ARG TAG

WORKDIR /src
ADD . .

RUN go build -ldflags "-X main.Version=${TAG}" -o /consensusdb

FROM ubuntu:18.04
WORKDIR /app

COPY --from=builder /consensusdb .

EXPOSE 4481 4482

CMD ["/app/consensusdb"]

