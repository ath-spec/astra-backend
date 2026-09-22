#!/usr/bin/env python3
"""
RAG Pipeline Module
Handles text chunking, embedding generation, and vector search
"""

import google.generativeai as genai
import faiss
import numpy as np
import os
import sys
from typing import List, Optional, Tuple

# Model configurations
EMBED_MODEL = "models/text-embedding-004"
LLM_MODEL = "gemini-2.5-flash"

def chunk_text(text: str, chunk_size: int = 500, overlap: int = 50) -> List[str]:
    """
    Split text into overlapping chunks for better context preservation
    
    Args:
        text: Input text to chunk
        chunk_size: Maximum words per chunk
        overlap: Number of words to overlap between chunks
        
    Returns:
        List of text chunks
    """
    words = text.split()
    chunks = []
    
    for i in range(0, len(words), chunk_size - overlap):
        chunk = " ".join(words[i:i + chunk_size])
        if chunk.strip():
            chunks.append(chunk)
    
    return chunks

def embed_texts(chunks: List[str]) -> np.ndarray:
    """
    Generate embeddings for text chunks using Gemini
    
    Args:
        chunks: List of text chunks to embed
        
    Returns:
        Numpy array of embeddings
    """
    try:
        result = genai.embed_content(
            model=EMBED_MODEL,
            content=chunks,
            task_type="retrieval_document"
        )
        return np.array([embedding for embedding in result["embedding"]])
    except Exception as e:
        print(f"Error generating embeddings: {e}", file=sys.stderr)
        return np.array([])

def build_index(chunks: List[str], embeddings: np.ndarray) -> Optional[faiss.Index]:
    """
    Build FAISS index for vector similarity search
    
    Args:
        chunks: List of text chunks
        embeddings: Numpy array of embeddings
        
    Returns:
        FAISS index or None if failed
    """
    if len(embeddings) == 0:
        return None
    
    try:
        dim = len(embeddings[0])
        index = faiss.IndexFlatL2(dim)
        index.add(embeddings.astype('float32'))
        return index
    except Exception as e:
        print(f"Error building FAISS index: {e}", file=sys.stderr)
        return None

def query_rag(index: faiss.Index, chunks: List[str], query: str, top_k: int = 5) -> str:
    """
    Retrieve relevant chunks using vector similarity search
    
    Args:
        index: FAISS index
        chunks: List of text chunks
        query: Search query
        top_k: Number of top results to return
        
    Returns:
        Concatenated relevant chunks
    """
    try:
        query_embedding = genai.embed_content(
            model=EMBED_MODEL,
            content=query,
            task_type="retrieval_query"
        )["embedding"]
        
        query_vector = np.array([query_embedding]).astype('float32')
        distances, indices = index.search(query_vector, min(top_k, len(chunks)))
        
        retrieved_chunks = [chunks[i] for i in indices[0] if i < len(chunks)]
        return "\n\n".join(retrieved_chunks)
    except Exception as e:
        print(f"Error in RAG query: {e}", file=sys.stderr)
        return ""

def generate_transactions(context: str) -> str:
    """
    Generate structured transaction data using Gemini
    
    Args:
        context: Retrieved context from RAG
        
    Returns:
        JSON string of transactions
    """
    try:
        prompt = f"""
You are a financial document parser. Extract all bank transactions from the following text and return them as a JSON array.

Text content:
{context}

Instructions:
1. Extract ALL transactions found in the text
2. For each transaction, provide:
   - date: Transaction date in YYYY-MM-DD format
   - description: Transaction description/merchant name
   - amount: Transaction amount (negative for debits, positive for credits)
   - balance: Account balance after transaction
   - transaction_type: "debit" or "credit"

3. Return ONLY a valid JSON array, no other text
4. If a field is missing, use null or appropriate default values
5. Ensure amounts are properly formatted as numbers

Example format:
[
  {{
    "date": "2025-01-15",
    "description": "UPI Transfer to Flipkart",
    "amount": -1499.50,
    "balance": 25342.30,
    "transaction_type": "debit"
  }}
]
"""

        model = genai.GenerativeModel(LLM_MODEL)
        response = model.generate_content(prompt)
        
        return response.text if response.text else ""
        
    except Exception as e:
        print(f"Error generating transactions: {e}", file=sys.stderr)
        return ""

if __name__ == "__main__":
    # Test the RAG pipeline
    if len(sys.argv) != 2:
        print("Usage: python rag_pipeline.py <text_file>", file=sys.stderr)
        sys.exit(1)
    
    text_file = sys.argv[1]
    
    try:
        with open(text_file, 'r', encoding='utf-8') as f:
            text = f.read()
        
        print("Chunking text...")
        chunks = chunk_text(text)
        print(f"Created {len(chunks)} chunks")
        
        print("Generating embeddings...")
        embeddings = embed_texts(chunks)
        print(f"Generated {len(embeddings)} embeddings")
        
        print("Building index...")
        index = build_index(chunks, embeddings)
        
        if index:
            print("Querying RAG...")
            result = query_rag(index, chunks, "bank transactions", top_k=3)
            print(f"Retrieved context: {len(result)} characters")
        else:
            print("Failed to build index")
            
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
