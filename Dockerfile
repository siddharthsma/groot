FROM golang:1.23-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/groot-api ./cmd/groot-api

FROM alpine:3.21

WORKDIR /app

COPY --from=build /out/groot-api /usr/local/bin/groot-api

EXPOSE 8081

ENTRYPOINT ["/usr/local/bin/groot-api"]
