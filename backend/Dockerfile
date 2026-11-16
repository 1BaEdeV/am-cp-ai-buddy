FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /api ./cmd/api && CGO_ENABLED=0 go build -o /seed-demo ./cmd/seed-demo

FROM alpine:3.22
COPY --from=build /api /usr/local/bin/api
COPY --from=build /seed-demo /usr/local/bin/seed-demo
USER nobody
EXPOSE 8080
ENTRYPOINT ["api"]
