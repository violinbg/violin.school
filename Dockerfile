# syntax=docker/dockerfile:1

FROM node:22-alpine AS ui-builder
WORKDIR /app/ui

COPY ui/package*.json ./
RUN npm ci

COPY ui/ ./
RUN npm run build

FROM golang:1.25-alpine AS go-builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY internal/ ./internal/
COPY migrations/ ./migrations/
COPY main.go ./

# go:embed expects the production UI at ui/dist/browser
COPY --from=ui-builder /app/ui/dist/browser ./ui/dist/browser

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/violin.school .

FROM alpine:3.22
WORKDIR /app

RUN addgroup -S app && adduser -S app -G app
RUN mkdir -p /data && chown -R app:app /data /app

COPY --from=go-builder /out/violin.school /app/violin.school

ENV PORT=8080
ENV DB_PATH=/data/violin.school.db

EXPOSE 8080
VOLUME ["/data"]

USER app
ENTRYPOINT ["/app/violin.school"]
