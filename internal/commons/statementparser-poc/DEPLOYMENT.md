# Bank Statement OCR - Deployment Guide

## Quick Start

### 1. System Requirements
- Go 1.21+
- Poppler-utils (pdftoppm, pdftotext)
- Tesseract OCR

### 2. Installation Commands

```bash
# macOS
brew install poppler tesseract

# Ubuntu/Debian
sudo apt-get update
sudo apt-get install poppler-utils tesseract-ocr

# CentOS/RHEL
sudo yum install poppler-utils tesseract
```

### 3. Project Setup

```bash
# Clone repository
git clone <your-repo-url>
cd bank-statement-ocr

# Create directories
mkdir -p backend/tmp/ocr

# Set permissions
chmod 755 backend/tmp/ocr
```

### 4. Environment Configuration

Create `.env` file in project root:

```env
# Server Configuration
SERVER_PORT=8080
SERVER_HOST=0.0.0.0

# File Upload Configuration
MAX_FILE_SIZE_MB=10
TEMP_DIR=tmp

# Logging Configuration
LOG_LEVEL=info
DEBUG_MODE=false
```

### 5. Build and Run

```bash
cd backend
go mod init bank-statement-ocr
go get github.com/joho/godotenv
go build -o fast_ocr_server .
./fast_ocr_server
```

## Production Deployment

### Docker Deployment

Create `Dockerfile`:

```dockerfile
FROM golang:1.21-alpine AS builder

# Install system dependencies
RUN apk add --no-cache poppler-utils tesseract-ocr

WORKDIR /app
COPY . .

# Build application
RUN go mod init bank-statement-ocr
RUN go get github.com/joho/godotenv
RUN go build -ldflags="-s -w" -o fast_ocr_server .

# Production image
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache poppler-utils tesseract-ocr

WORKDIR /app

# Copy binary and create directories
COPY --from=builder /app/fast_ocr_server .
COPY --from=builder /app/.env .
RUN mkdir -p tmp/ocr

# Set permissions
RUN chmod 755 fast_ocr_server
RUN chmod 755 tmp/ocr

EXPOSE 8080

CMD ["./fast_ocr_server"]
```

Build and run:

```bash
docker build -t bank-statement-ocr .
docker run -p 8080:8080 bank-statement-ocr
```

### Systemd Service

Create `/etc/systemd/system/bank-statement-ocr.service`:

```ini
[Unit]
Description=Bank Statement OCR Service
After=network.target

[Service]
Type=simple
User=ocr
Group=ocr
WorkingDirectory=/opt/bank-statement-ocr
ExecStart=/opt/bank-statement-ocr/fast_ocr_server
Restart=always
RestartSec=5
Environment=LOG_LEVEL=info
Environment=DEBUG_MODE=false

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable bank-statement-ocr
sudo systemctl start bank-statement-ocr
```

### Nginx Reverse Proxy

Create `/etc/nginx/sites-available/bank-statement-ocr`:

```nginx
server {
    listen 80;
    server_name your-domain.com;

    client_max_body_size 10M;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

Enable site:

```bash
sudo ln -s /etc/nginx/sites-available/bank-statement-ocr /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

## Testing

### Health Check
```bash
curl http://localhost:8080/health
```

### Upload and Extract
```bash
# Upload file
curl -X POST -F "file=@sample-statement.pdf" http://localhost:8080/upload

# Extract transactions
curl -X GET "http://localhost:8080/fast-extract?file=tmp/sample-statement.pdf"
```

## Monitoring

### Log Monitoring
```bash
# View logs
journalctl -u bank-statement-ocr -f

# Docker logs
docker logs -f bank-statement-ocr
```

### Performance Monitoring
```bash
# Check process
ps aux | grep fast_ocr_server

# Check memory usage
top -p $(pgrep fast_ocr_server)

# Check disk usage
du -sh backend/tmp/
```

## Troubleshooting

### Common Issues

1. **Permission Denied**
   ```bash
   chmod 755 backend/tmp/ocr
   chown -R ocr:ocr backend/tmp/
   ```

2. **Port Already in Use**
   ```bash
   lsof -ti:8080 | xargs kill -9
   ```

3. **OCR Accuracy Issues**
   - Increase resolution: Change `-r 300` to `-r 600`
   - Try different PSM modes: `--psm 6` to `--psm 3`

4. **Memory Issues**
   - Monitor temp directory size
   - Implement file cleanup cron job
   - Increase server memory

### Debug Mode

Enable debug logging:

```env
DEBUG_MODE=true
LOG_LEVEL=debug
```

Restart service:

```bash
sudo systemctl restart bank-statement-ocr
```

## Security Hardening

### File Permissions
```bash
# Secure directories
chmod 700 backend/tmp/ocr
chown -R ocr:ocr backend/

# Secure binary
chmod 755 fast_ocr_server
chown root:root fast_ocr_server
```

### Firewall Rules
```bash
# Allow only necessary ports
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw deny 8080/tcp
```

### Process Isolation
```bash
# Run as non-root user
sudo useradd -r -s /bin/false ocr
sudo chown -R ocr:ocr /opt/bank-statement-ocr
```

## Scaling

### Horizontal Scaling
- Use load balancer (nginx/HAProxy)
- Multiple instances behind reverse proxy
- Shared storage for temp files (NFS/S3)

### Vertical Scaling
- Increase server resources
- Optimize OCR settings
- Implement caching layer

## Backup and Recovery

### Configuration Backup
```bash
# Backup configuration
tar -czf bank-statement-ocr-config.tar.gz .env backend/
```

### Data Backup
```bash
# Backup uploaded files
rsync -av backend/tmp/ /backup/tmp/
```

### Recovery
```bash
# Restore from backup
tar -xzf bank-statement-ocr-config.tar.gz
sudo systemctl restart bank-statement-ocr
```

---

**For support and questions, please refer to the main README.md file.**
