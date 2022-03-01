FROM golang:1.17-alpine

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY *.go ./
RUN go build -o /ipfs-check-pp

EXPOSE 3333
CMD [ "/ipfs-check-pp" ]
