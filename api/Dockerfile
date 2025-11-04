# syntax=docker/dockerfile:1

FROM golang:1.25.3 as builder

ENV GOTOOLCHAIN=auto

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/fiatrails ./cmd/server

FROM gcr.io/distroless/base-debian12

ENV API_HTTP_PORT=3000
EXPOSE 3000

COPY --from=builder /bin/fiatrails /bin/fiatrails

ENTRYPOINT ["/bin/fiatrails"]
