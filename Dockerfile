FROM golang:latest
WORKDIR /app/forum
COPY . .
RUN go mod download
RUN go build -o main
CMD ["go run ."]