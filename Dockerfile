# Control My Server Telegram Bot Dockerfile
# Build and run the bot in a container

# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy source code
COPY go.mod go.sum ./
COPY src/ ./src/

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags "-X main.AppVersion=$(cat VERSION)" \
    -o control_my_server_bot ./src

# Runtime stage
FROM alpine:3.20

WORKDIR /opt/control_my_server_bot

# Install runtime dependencies
RUN apk add --no-cache \
    bash \
    systemd \
    docker-cli \
    procps \
    && rm -rf /var/cache/apk/*

# Create dedicated user
RUN adduser -D -H -s /bin/false control_my_server_bot_user

# Copy binary from builder
COPY --from=builder /app/control_my_server_bot .
COPY --from=builder /app/VERSION .
COPY apply_update.sh .
COPY .env.example .

# Copy systemd service file
COPY control_my_server_bot.service /usr/lib/systemd/system/

# Copy scripts
RUN mkdir -p /scripts
COPY scripts/ /scripts/

# Set permissions
RUN chown -R control_my_server_bot_user:control_my_server_bot_user /opt/control_my_server_bot
RUN chmod 700 /opt/control_my_server_bot/control_my_server_bot
RUN chmod 700 /opt/control_my_server_bot/apply_update.sh
RUN chmod 600 /opt/control_my_server_bot/.env.example

# Switch to non-root user
USER control_my_server_bot_user

# Set working directory
WORKDIR /opt/control_my_server_bot

# Expose port (Telegram Bot API uses HTTPS, no port needed for bot itself)
# The bot connects out to Telegram, it doesn't listen on any port

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD test -f /opt/control_my_server_bot/control_my_server_bot || exit 1

# Default command (will be overridden by entrypoint)
ENTRYPOINT ["/opt/control_my_server_bot/control_my_server_bot"]
CMD ["--help"]
