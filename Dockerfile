FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY main.go ./
COPY web/ ./web/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /gharkiraya .

FROM alpine:3.22
RUN addgroup -g 10001 app && adduser -D -u 10001 -G app app && mkdir /data && chown app:app /data
COPY --from=build /gharkiraya /usr/local/bin/gharkiraya
USER app
ENV ADDR=0.0.0.0:8080 DATA_FILE=/data/state.json DEMO_MODE=false
EXPOSE 8080
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=3s CMD wget -q -O /dev/null http://127.0.0.1:8080/api/health || exit 1
CMD ["gharkiraya"]
