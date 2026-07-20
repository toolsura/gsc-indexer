FROM golang:1.25

WORKDIR /src

# Pull deps first so the (rarely changing) module files are cached.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the binary onto PATH.
RUN go build -o /usr/local/bin/gsc-indexer .

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
