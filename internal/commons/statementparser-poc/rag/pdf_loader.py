#!/usr/bin/env python3
"""
PDF Text Extraction Module
Handles PDF parsing and text extraction using PyMuPDF
"""

import fitz  # PyMuPDF
import sys
from typing import Optional

def extract_text_from_pdf(pdf_path: str) -> Optional[str]:
    """
    Extract text from PDF using PyMuPDF
    
    Args:
        pdf_path: Path to the PDF file
        
    Returns:
        Extracted text as string, or None if extraction fails
    """
    try:
        doc = fitz.open(pdf_path)
        text = ""
        
        for page_num in range(len(doc)):
            page = doc[page_num]
            text += page.get_text("text") + "\n"
        
        doc.close()
        return text.strip()
        
    except Exception as e:
        print(f"Error extracting text from PDF '{pdf_path}': {e}", file=sys.stderr)
        return None

def extract_text_with_metadata(pdf_path: str) -> dict:
    """
    Extract text and metadata from PDF
    
    Args:
        pdf_path: Path to the PDF file
        
    Returns:
        Dictionary containing text and metadata
    """
    try:
        doc = fitz.open(pdf_path)
        
        result = {
            "text": "",
            "page_count": len(doc),
            "metadata": doc.metadata,
            "pages": []
        }
        
        for page_num in range(len(doc)):
            page = doc[page_num]
            page_text = page.get_text("text")
            result["text"] += page_text + "\n"
            result["pages"].append({
                "page_number": page_num + 1,
                "text": page_text,
                "rect": page.rect
            })
        
        doc.close()
        result["text"] = result["text"].strip()
        return result
        
    except Exception as e:
        print(f"Error extracting text with metadata from PDF '{pdf_path}': {e}", file=sys.stderr)
        return {"text": "", "page_count": 0, "metadata": {}, "pages": []}

if __name__ == "__main__":
    if len(sys.argv) != 2:
        print("Usage: python pdf_loader.py <pdf_path>", file=sys.stderr)
        sys.exit(1)
    
    pdf_path = sys.argv[1]
    text = extract_text_from_pdf(pdf_path)
    
    if text:
        print(f"Extracted {len(text)} characters from PDF")
        print("First 500 characters:")
        print(text[:500])
    else:
        print("Failed to extract text from PDF")
