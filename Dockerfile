FROM shvid/ubuntu-golang as builder

ARG TAG

WORKDIR /go/src/github.com/consensusdb/consensusdb
ADD . .

RUN sed -i "s/%TAG%/${TAG}/g" main.go && \
    go build -o /consensusdb

FROM ubuntu:18.04
WORKDIR /app

COPY --from=builder /consensusdb .

EXPOSE 4481 4482

CMD ["/app/consensusdb"]

