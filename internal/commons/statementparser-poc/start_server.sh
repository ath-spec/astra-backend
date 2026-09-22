#!/bin/bash

# Bank Statement OCR Pipeline - Quick Start Script

echo "🏦 Bank Statement OCR Pipeline - Quick Start"
echo "============================================="

# Check if server is already running
if curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo "✅ Server is already running on http://localhost:8080"
    echo ""
    echo "Available commands:"
    echo "  Upload PDF:    curl -X POST -F \"pdf=@your_file.pdf\" http://localhost:8080/upload"
    echo "  Fast Extract:  curl \"http://localhost:8080/fast-extract?file=tmp/filename.pdf\""
    echo "  Health Check:  curl http://localhost:8080/health"
    echo ""
    echo "Press Ctrl+C to stop the server"
    exit 0
fi

# Start the server
echo "🚀 Starting server..."
cd backend
./fast_ocr_server &
SERVER_PID=$!

# Wait for server to start
echo "⏳ Waiting for server to start..."
sleep 3

# Test if server is running
if curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo "✅ Server started successfully!"
    echo ""
    echo "🌐 Server running on: http://localhost:8080"
    echo ""
    echo "📋 Available endpoints:"
    echo "  GET  /health         - Health check"
    echo "  POST /upload         - Upload PDF file"
    echo "  GET  /extract        - Extract transactions (RAG)"
    echo "  GET  /fast-extract   - Fast OCR extraction"
    echo ""
    echo "📤 Example usage:"
    echo "  # Upload a PDF"
    echo "  curl -X POST -F \"pdf=@statement.pdf\" http://localhost:8080/upload"
    echo ""
    echo "  # Extract transactions"
    echo "  curl \"http://localhost:8080/fast-extract?file=tmp/statement_TIMESTAMP_filename.pdf\""
    echo ""
    echo "Press Ctrl+C to stop the server"
    
    # Keep script running and handle Ctrl+C
    trap "echo ''; echo '🛑 Stopping server...'; kill $SERVER_PID; exit 0" INT
    wait $SERVER_PID
else
    echo "❌ Failed to start server"
    kill $SERVER_PID 2>/dev/null
    exit 1
fi

