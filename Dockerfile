FROM golang:1.24 AS builder
WORKDIR /app
COPY ["go.*", "./"]
RUN go mod download

COPY [".", "./"]
RUN --mount=type=cache,target=/root/.cache/go-build make release build
WORKDIR /dist
RUN cp /app/app ./app
RUN ldd app | tr -s '[:blank:]' '\n' | grep '^/' | \
    xargs -I % sh -c 'mkdir -p lib/$(dirname ./%); cp % ./lib/%;';
RUN if [ "$(uname -m)" = "x86_64" ]; then \
      mkdir -p lib64 && cp /lib64/ld-linux-x86-64.so.2 lib64/; \
    fi;
RUN mkdir -p tmp && chmod 1777 tmp

FROM scratch
COPY --chown=0:0 --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --chown=0:0 --from=builder /dist/* /
COPY --chown=0:0 --from=builder /dist/tmp /tmp
USER 65534
ENTRYPOINT ["/app"]
