FROM golang:1.11 as build

RUN mkdir -p /go/src/github.com/wrouesnel/reverse_exporter
WORKDIR /go/src/github.com/wrouesnel/reverse_exporter
COPY . .

RUN go build ./cmd/reverse_exporter/

ENTRYPOINT [ "./reverse_exporter" ]
