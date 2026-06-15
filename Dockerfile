FROM shvid/ubuntu-golang as builder

ARG TAG

WORKDIR /src
ADD . .

RUN go build -ldflags "-X main.Version=${TAG}" -o /consensusdb

FROM ubuntu:18.04
WORKDIR /app

COPY --from=builder /consensusdb .

EXPOSE 8441 8442

CMD ["/app/consensusdb", "run"]

