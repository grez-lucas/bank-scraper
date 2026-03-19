# Build stage
FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o credmgr ./cmd/credmgr

# Runtime stage
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /app/credmgr /usr/local/bin/credmgr
EXPOSE 8081
ENTRYPOINT ["credmgr"]
CMD ["serve"]
