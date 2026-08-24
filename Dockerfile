FROM golang:1.26-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /tuma ./cmd/tuma

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /tuma /usr/local/bin/tuma
COPY migrations /app/migrations
ENV MIGRATIONS_PATH=file:///app/migrations
EXPOSE 8080
ENTRYPOINT ["tuma"]
