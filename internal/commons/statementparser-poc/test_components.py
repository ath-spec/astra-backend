#!/usr/bin/env python3
"""
Simple test script to verify the Python components work
"""

import sys
import os

# Add the rag directory to the path
sys.path.append(os.path.join(os.path.dirname(__file__), 'rag'))

def test_imports():
    """Test if all required modules can be imported"""
    try:
        import google.generativeai as genai
        print("✅ google.generativeai imported successfully")
    except ImportError as e:
        print(f"❌ Failed to import google.generativeai: {e}")
        return False
    
    try:
        import fitz  # PyMuPDF
        print("✅ PyMuPDF (fitz) imported successfully")
    except ImportError as e:
        print(f"❌ Failed to import PyMuPDF: {e}")
        return False
    
    try:
        import faiss
        print("✅ FAISS imported successfully")
    except ImportError as e:
        print(f"❌ Failed to import FAISS: {e}")
        return False
    
    try:
        import numpy as np
        print("✅ NumPy imported successfully")
    except ImportError as e:
        print(f"❌ Failed to import NumPy: {e}")
        return False
    
    return True

def test_pdf_loader():
    """Test PDF loader functionality"""
    try:
        from pdf_loader import extract_text_from_pdf
        print("✅ PDF loader module imported successfully")
        return True
    except ImportError as e:
        print(f"❌ Failed to import PDF loader: {e}")
        return False

def test_rag_pipeline():
    """Test RAG pipeline functionality"""
    try:
        from rag_pipeline import chunk_text, embed_texts, build_index
        print("✅ RAG pipeline module imported successfully")
        
        # Test chunking
        test_text = "This is a test text for chunking. " * 100
        chunks = chunk_text(test_text, chunk_size=50, overlap=10)
        print(f"✅ Text chunking works: {len(chunks)} chunks created")
        
        return True
    except ImportError as e:
        print(f"❌ Failed to import RAG pipeline: {e}")
        return False
    except Exception as e:
        print(f"❌ RAG pipeline test failed: {e}")
        return False

def main():
    print("🧪 Testing Bank Statement RAG System Components")
    print("=" * 50)
    
    all_tests_passed = True
    
    print("\n📦 Testing imports...")
    if not test_imports():
        all_tests_passed = False
    
    print("\n📄 Testing PDF loader...")
    if not test_pdf_loader():
        all_tests_passed = False
    
    print("\n🧠 Testing RAG pipeline...")
    if not test_rag_pipeline():
        all_tests_passed = False
    
    print("\n" + "=" * 50)
    if all_tests_passed:
        print("🎉 All tests passed! The system is ready to use.")
        print("\nNext steps:")
        print("1. Set your Gemini API key: export GEMINI_API_KEY='your_key_here'")
        print("2. Install dependencies: cd rag && pip install -r requirements.txt")
        print("3. Start the server: cd backend && go run .")
    else:
        print("❌ Some tests failed. Please check the errors above.")
        print("\nTo install missing dependencies:")
        print("cd rag && pip install -r requirements.txt")

if __name__ == "__main__":
    main()
