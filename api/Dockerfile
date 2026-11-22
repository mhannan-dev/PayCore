FROM golang:1.27-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S paycore \
    && adduser -S -G paycore paycore
WORKDIR /app
COPY --from=build /out/server ./server
RUN mkdir -p uploads && chown -R paycore:paycore /app
USER paycore
EXPOSE 8080
ENTRYPOINT ["/app/server"]
