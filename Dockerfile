# syntax=docker/dockerfile:1

FROM golang:1.24-alpine

WORKDIR /app

RUN apk update && \
	apk add --no-cache make

COPY . .
RUN go mod download
RUN make build-all

RUN apk --no-cache add ca-certificates
# RUN apk --no-cache add chromium

RUN chmod +x scripts/docker-entrypoint.sh

ENTRYPOINT [ "./scripts/docker-entrypoint.sh" ]
CMD [ "./target/base58-website" ]
