FROM golang:1.27.3-alpine AS build
WORKDIR /src/apps/api
COPY apps/api/go.mod apps/api/go.sum ./
RUN go mod download
COPY apps/api ./
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api && CGO_ENABLED=0 go build -o /out/migrate ./cmd/migrate

FROM alpine:3.22
WORKDIR /app
RUN apk add --no-cache ca-certificates
COPY --from=build /out/api /app/api
COPY --from=build /out/migrate /app/migrate
COPY infrastructure/migrations /app/migrations
USER 10001:10001
EXPOSE 8080
CMD ["/app/api"]
