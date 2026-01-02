FROM golang:1.25-alpine AS builder
WORKDIR /app

COPY packages/core/go.mod packages/core/go.sum ./packages/core/
COPY apps/parser/go.mod ./apps/parser/

WORKDIR /app/apps/parser
ENV GOWORK=off
RUN go mod download -x

WORKDIR /app
COPY packages/core ./packages/core
COPY apps/parser ./apps/parser

WORKDIR /app/apps/parser
RUN CGO_ENABLED=0 go build -o /parser .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /parser /parser
EXPOSE 8080
CMD ["/parser"]
