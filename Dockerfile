# Builds the ProxyConnectorBot binary and packages it into a small runtime image.

FROM golang:1.27-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/pcb ./cmd/pcb

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/pcb /usr/local/bin/pcb

EXPOSE 8080
ENTRYPOINT ["pcb"]
