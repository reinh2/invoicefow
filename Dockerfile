FROM node:22-bookworm-slim AS web
WORKDIR /web
COPY web/package.json web/package-lock.json* ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/invoiceflow-api ./cmd/api && CGO_ENABLED=0 go build -trimpath -o /out/invoiceflow-worker ./cmd/worker && mkdir -p /out/data && touch /out/data/.keep
RUN CGO_ENABLED=0 go build -trimpath -o /out/invoiceflow-receiver ./cmd/receiver

FROM debian:12.11-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        poppler-utils=22.12.0-2+deb12u2 \
        tesseract-ocr=5.3.0-2 \
        ca-certificates=20230311+deb12u1 \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/invoiceflow-api /app/invoiceflow-api
COPY --from=build /out/invoiceflow-worker /app/invoiceflow-worker
COPY --from=build /out/invoiceflow-receiver /app/invoiceflow-receiver
COPY db/migrations /app/db/migrations
COPY --from=web /web/dist /app/web
COPY --from=build /out/data /data
RUN useradd --system --uid 65532 --create-home invoiceflow \
    && chown -R invoiceflow:invoiceflow /app /data
USER invoiceflow:invoiceflow
CMD ["/app/invoiceflow-api"]
