FROM golang:1.26-alpine AS build
RUN apk add --no-cache make git bash gcc musl-dev linux-headers
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make build

FROM alpine:3.20
RUN apk add --no-cache ca-certificates jq bash curl
COPY --from=build /src/build/harbord /usr/local/bin/harbord
COPY scripts/localnet.sh scripts/smoke.sh /usr/local/bin/
EXPOSE 26656 26657 1317 9090
ENTRYPOINT ["harbord"]
