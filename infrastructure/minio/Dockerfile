FROM golang:1.27.3-alpine AS build
RUN apk add --no-cache git
WORKDIR /src
# Latest published community release; source pinned because official images are unavailable.
ADD https://github.com/minio/minio/archive/9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a.tar.gz /tmp/minio.tar.gz
RUN tar -xzf /tmp/minio.tar.gz --strip-components=1 && CGO_ENABLED=0 go build -o /out/minio .
FROM alpine:3.22
RUN apk add --no-cache ca-certificates curl && mkdir /data && chown 10001:10001 /data
COPY --from=build /out/minio /usr/local/bin/minio
COPY --from=build /src/LICENSE /licenses/minio-LICENSE
USER 10001:10001
EXPOSE 9000 9001
ENTRYPOINT ["minio"]
CMD ["server", "/data", "--console-address", ":9001"]
