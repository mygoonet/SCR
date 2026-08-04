FROM golang:1.26-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /scr .

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates fonts-liberation libasound2 libatk-bridge2.0-0 libatk1.0-0 \
    libcups2 libdbus-1-3 libdrm2 libgbm1 libgtk-3-0 libpcsclite1 libxft2 \
    libnspr4 libnss3 libx11-xcb1 libxcomposite1 \
    libxdamage1 libxrandr2 xdg-utils \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /scr /usr/local/bin/scr

RUN mkdir -p /home/app/.config/chromium-gost-scrp

ENV CHROME_PATH=/opt/chromium-gost/chromium-gost
ENV USER_DATA_DIR=/home/app/.config/chromium-gost-scrp
ENV CERT_USER="Сичкарук Евгений Александрович"

ENTRYPOINT ["scr"]
