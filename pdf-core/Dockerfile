FROM golang:1.25-alpine AS builder
RUN apk add --no-cache make
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o dcspdfcore .

FROM alpine:3.20
RUN addgroup -S app && adduser -S -G app app
WORKDIR /app
COPY --from=builder /app/dcspdfcore ./dcspdfcore
USER app
EXPOSE 8080
ENV DCS_PDF_CORE_ADDR=0.0.0.0:8080
ENTRYPOINT ["./dcspdfcore"]
