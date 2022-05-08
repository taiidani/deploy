FROM alpine:3.15

COPY deploy /app

LABEL org.opencontainers.image.source=https://github.com/taiidani/deploy

EXPOSE 8082
ENTRYPOINT ["/app"]
