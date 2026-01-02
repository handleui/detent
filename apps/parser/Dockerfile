FROM golang:1.22-alpine AS builder
WORKDIR /app

# Copy workspace and module files
COPY go.work go.work.sum ./
COPY packages/core/go.mod packages/core/go.sum ./packages/core/
COPY apps/parser/go.mod ./apps/parser/

# Download dependencies using workspace
RUN go mod download -x

# Copy source
COPY packages/core ./packages/core
COPY apps/parser ./apps/parser

# Build
RUN cd apps/parser && CGO_ENABLED=0 go build -o /parser .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /parser /parser
EXPOSE 8080
CMD ["/parser"]
