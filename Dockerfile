FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /pgadmin-cnpg-discovery ./cmd

FROM gcr.io/distroless/static:nonroot

COPY --from=builder /pgadmin-cnpg-discovery /pgadmin-cnpg-discovery

USER nonroot:nonroot

ENTRYPOINT ["/pgadmin-cnpg-discovery"]
