FROM alpine:3.17.2

COPY deploy /app

LABEL org.opencontainers.image.source=https://github.com/taiidani/deploy

EXPOSE 8082
ENTRYPOINT ["/app"]
