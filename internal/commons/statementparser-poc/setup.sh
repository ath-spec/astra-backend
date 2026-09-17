#!/bin/bash

# Bank Statement RAG System Setup Script
# This script helps set up the environment configuration

set -e

echo "🏦 Bank Statement RAG System Setup"
echo "=================================="

# Check if .env already exists
if [ -f ".env" ]; then
    echo "⚠️  .env file already exists"
    read -p "Do you want to overwrite it? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Setup cancelled"
        exit 0
    fi
fi

echo "📝 Creating .env file..."

# Get Gemini API key from user
echo ""
echo "Please enter your Gemini API key:"
echo "(Get it from: https://aistudio.google.com/app/apikey)"
read -p "GEMINI_API_KEY: " GEMINI_API_KEY

if [ -z "$GEMINI_API_KEY" ]; then
    echo "❌ Error: Gemini API key is required"
    exit 1
fi

# Create .env file
cat > .env << EOF
# Bank Statement RAG System Environment Configuration
# Generated on $(date)

# Gemini API Configuration
GEMINI_API_KEY=$GEMINI_API_KEY

# Server Configuration
SERVER_PORT=8080
SERVER_HOST=localhost

# File Upload Configuration
MAX_FILE_SIZE_MB=10
TEMP_DIR=tmp

# RAG Configuration
CHUNK_SIZE=500
CHUNK_OVERLAP=50
TOP_K_RETRIEVAL=8

# Logging Configuration
LOG_LEVEL=info
DEBUG_MODE=true
EOF

echo "✅ .env file created successfully!"

# Test the configuration
echo ""
echo "🧪 Testing configuration..."
if source venv/bin/activate 2>/dev/null; then
    python3 rag/env_config.py
    echo "✅ Configuration test passed!"
else
    echo "⚠️  Virtual environment not found. Please run:"
    echo "   python3 -m venv venv"
    echo "   source venv/bin/activate"
    echo "   pip install -r rag/requirements.txt"
fi

echo ""
echo "🎉 Setup complete!"
echo ""
echo "Next steps:"
echo "1. Activate virtual environment: source venv/bin/activate"
echo "2. Start the server: cd backend && go run ."
echo "3. Test the system: curl http://localhost:8080/health"
echo ""
echo "Your Gemini API key is configured and ready to use! 🚀"
