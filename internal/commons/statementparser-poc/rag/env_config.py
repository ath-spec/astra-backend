#!/usr/bin/env python3
"""
Environment configuration loader for Bank Statement RAG System
"""

import os
from typing import Optional

def load_env_file(env_path: str = ".env") -> None:
    """
    Load environment variables from a .env file
    
    Args:
        env_path: Path to the .env file
    """
    if not os.path.exists(env_path):
        return
    
    with open(env_path, 'r') as f:
        for line in f:
            line = line.strip()
            if line and not line.startswith('#') and '=' in line:
                key, value = line.split('=', 1)
                key = key.strip()
                value = value.strip().strip('"').strip("'")
                os.environ[key] = value

def get_env(key: str, default: Optional[str] = None) -> Optional[str]:
    """
    Get environment variable with optional default
    
    Args:
        key: Environment variable name
        default: Default value if key not found
        
    Returns:
        Environment variable value or default
    """
    return os.environ.get(key, default)

def get_env_int(key: str, default: int = 0) -> int:
    """
    Get environment variable as integer with default
    
    Args:
        key: Environment variable name
        default: Default value if key not found or invalid
        
    Returns:
        Environment variable value as integer or default
    """
    try:
        return int(os.environ.get(key, str(default)))
    except (ValueError, TypeError):
        return default

def get_env_bool(key: str, default: bool = False) -> bool:
    """
    Get environment variable as boolean with default
    
    Args:
        key: Environment variable name
        default: Default value if key not found or invalid
        
    Returns:
        Environment variable value as boolean or default
    """
    value = os.environ.get(key, str(default)).lower()
    return value in ('true', '1', 'yes', 'on')

# Load environment variables on import
load_env_file()

# Configuration constants
GEMINI_API_KEY = get_env("GEMINI_API_KEY")
SERVER_PORT = get_env_int("SERVER_PORT", 8080)
SERVER_HOST = get_env("SERVER_HOST", "localhost")
MAX_FILE_SIZE_MB = get_env_int("MAX_FILE_SIZE_MB", 10)
TEMP_DIR = get_env("TEMP_DIR", "tmp")
CHUNK_SIZE = get_env_int("CHUNK_SIZE", 500)
CHUNK_OVERLAP = get_env_int("CHUNK_OVERLAP", 50)
TOP_K_RETRIEVAL = get_env_int("TOP_K_RETRIEVAL", 8)
LOG_LEVEL = get_env("LOG_LEVEL", "info")
DEBUG_MODE = get_env_bool("DEBUG_MODE", False)

if __name__ == "__main__":
    print("Environment Configuration:")
    print(f"GEMINI_API_KEY: {'***masked***' if GEMINI_API_KEY else 'Not set'}")
    print(f"SERVER_PORT: {SERVER_PORT}")
    print(f"SERVER_HOST: {SERVER_HOST}")
    print(f"MAX_FILE_SIZE_MB: {MAX_FILE_SIZE_MB}")
    print(f"TEMP_DIR: {TEMP_DIR}")
    print(f"CHUNK_SIZE: {CHUNK_SIZE}")
    print(f"CHUNK_OVERLAP: {CHUNK_OVERLAP}")
    print(f"TOP_K_RETRIEVAL: {TOP_K_RETRIEVAL}")
    print(f"LOG_LEVEL: {LOG_LEVEL}")
    print(f"DEBUG_MODE: {DEBUG_MODE}")
