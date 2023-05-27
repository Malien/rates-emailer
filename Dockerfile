from golang:1.17.8-alpine as builder
WORKDIR /rates-emailer
COPY go.mod go.sum .
RUN go mod download
COPY main.go rate_api.go sendmail.go emaildb.go .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags '-extldflags "-static"' -o rates-emailer

from scratch
ENV APP_MODE=prod
WORKDIR /rates-emailer
COPY conf/config.toml conf/config.prod.toml conf/
COPY --from=builder /rates-emailer/rates-emailer .
ENTRYPOINT ["./rates-emailer"]
