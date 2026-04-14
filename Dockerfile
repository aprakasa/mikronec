FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
  -a -installsuffix cgo \
  -o /mikronec ./cmd/server

FROM gcr.io/distroless/static-debian12 AS final
WORKDIR /
COPY --from=builder /mikronec /mikronec
ENV PORT=8080
EXPOSE 8080
ENTRYPOINT ["/mikronec"]
