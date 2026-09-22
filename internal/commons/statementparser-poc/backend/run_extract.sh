#!/bin/bash
# Wrapper script to run Python extraction from Go backend

# Get the directory where this script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# Navigate to project root
cd "$SCRIPT_DIR/.."

# Load environment variables
if [ -f .env ]; then
    export $(grep -v '^#' .env | xargs)
fi

# Convert the file path to absolute if it's relative
FILE_PATH="$1"
if [[ ! "$FILE_PATH" = /* ]]; then
    # The file is relative to the backend directory, so we need to go back to backend
    FILE_PATH="$SCRIPT_DIR/$FILE_PATH"
fi

# Activate virtual environment and run Python script
source venv/bin/activate
python3 rag/extract.py "$FILE_PATH"

