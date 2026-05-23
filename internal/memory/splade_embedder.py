#!/usr/bin/env python3
"""
SPLADE Vector Embedder Tool

Generates SPLADE sparse vectors for text using pre-trained models.
Supports model download from ModelScope or HuggingFace.

Usage:
    python main.py -text "your text here" [-model "naver/splade-v3"] [-source "modelscope"]
    python main.py -batch -file input.jsonl [-model "naver/splade-v3"]
    python main.py -download [-model "naver/splade-v3"] [-source "modelscope"] [-cache_dir ".splade_models"]
"""

import argparse
import json
import sys
import os
from pathlib import Path
from typing import List, Dict, Optional, Tuple


def download_model(model_name: str, source: str, cache_dir: str) -> str:
    """
    Download model from ModelScope or HuggingFace.
    
    Args:
        model_name: Model name/ID
        source: "modelscope" or "huggingface"
        cache_dir: Local cache directory
        
    Returns:
        Path to downloaded model
    """
    cache_path = Path(cache_dir) / model_name.replace("/", "_")
    
    if cache_path.exists() and any(cache_path.iterdir()):
        print(f"Model already exists at {cache_path}", file=sys.stderr)
        return str(cache_path)
    
    cache_path.mkdir(parents=True, exist_ok=True)
    
    try:
        if source == "modelscope":
            print(f"Downloading model {model_name} from ModelScope...", file=sys.stderr)
            from modelscope import snapshot_download
            
            model_path = snapshot_download(
                model_id=model_name,
                cache_dir=str(cache_path),
                revision="master"
            )
            print(f"Model downloaded to {model_path}", file=sys.stderr)
            return model_path
            
        elif source == "huggingface":
            print(f"Downloading model {model_name} from HuggingFace...", file=sys.stderr)
            from huggingface_hub import snapshot_download
            
            model_path = snapshot_download(
                repo_id=model_name,
                cache_dir=str(cache_path),
                local_dir=str(cache_path / "model")
            )
            print(f"Model downloaded to {model_path}", file=sys.stderr)
            return model_path
            
        else:
            raise ValueError(f"Unsupported source: {source}. Use 'modelscope' or 'huggingface'")
            
    except ImportError as e:
        print(f"Error: Missing dependency - {e}", file=sys.stderr)
        print("Install required packages:", file=sys.stderr)
        if source == "modelscope":
            print("  pip install modelscope torch transformers", file=sys.stderr)
        else:
            print("  pip install huggingface_hub torch transformers", file=sys.stderr)
        sys.exit(1)
    except Exception as e:
        print(f"Error downloading model: {e}", file=sys.stderr)
        sys.exit(1)


def load_model(model_path: str):
    """
    Load SPLADE model and tokenizer.
    
    Args:
        model_path: Path to the model
        
    Returns:
        Tuple of (model, tokenizer)
    """
    try:
        from transformers import AutoModelForMaskedLM, AutoTokenizer
        import torch
        
        print(f"Loading model from {model_path}...", file=sys.stderr)
        
        tokenizer = AutoTokenizer.from_pretrained(model_path)
        model = AutoModelForMaskedLM.from_pretrained(model_path)
        model.eval()
        
        # Use GPU if available
        device = torch.device("cuda" if torch.cuda.is_available() else "cpu")
        model.to(device)
        
        print(f"Model loaded on {device}", file=sys.stderr)
        return model, tokenizer, device
        
    except ImportError as e:
        print(f"Error: Missing dependency - {e}", file=sys.stderr)
        print("Install required packages: pip install torch transformers", file=sys.stderr)
        sys.exit(1)
    except Exception as e:
        print(f"Error loading model: {e}", file=sys.stderr)
        sys.exit(1)


def generate_splade_vector(
    text: str,
    model,
    tokenizer,
    device,
    max_length: int = 256
) -> Dict[int, float]:
    """
    Generate SPLADE sparse vector for text.
    
    Args:
        text: Input text
        model: SPLADE model
        tokenizer: Tokenizer
        device: Device to run inference on
        max_length: Maximum sequence length
        
    Returns:
        Dictionary mapping token indices to weights (sparse vector)
    """
    import torch
    
    # Tokenize input
    inputs = tokenizer(
        text,
        padding=True,
        truncation=True,
        max_length=max_length,
        return_tensors="pt"
    ).to(device)
    
    # Generate embeddings
    with torch.no_grad():
        outputs = model(**inputs)
        logits = outputs.logits  # (batch_size, seq_len, vocab_size)
        
        # Apply ReLU and max pooling over sequence dimension
        # This is the SPLADE mechanism: max over sequence for each vocab token
        relu_logits = torch.relu(logits)
        sparse_vector = torch.max(relu_logits, dim=1).squeeze()  # (vocab_size,)
        
        # Convert to dictionary (only non-zero values)
        vector_dict = {}
        for idx, weight in enumerate(sparse_vector.cpu().numpy()):
            if weight > 0:
                vector_dict[int(idx)] = float(weight)
    
    return vector_dict


def format_sparse_vector(vector_dict: Dict[int, float], dim: int = 30522) -> dict:
    """
    Format sparse vector for JSON output.
    
    Args:
        vector_dict: Dictionary mapping indices to values
        dim: Total vector dimension
        
    Returns:
        Formatted dictionary with indices, values, and dim
    """
    if not vector_dict:
        return {"indices": [], "values": [], "dim": dim}
    
    # Sort by index
    sorted_items = sorted(vector_dict.items())
    indices = [idx for idx, _ in sorted_items]
    values = [val for _, val in sorted_items]
    
    return {
        "indices": indices,
        "values": [round(v, 6) for v in values],  # Round to save space
        "dim": dim
    }


def main():
    parser = argparse.ArgumentParser(
        description="SPLADE Vector Embedder - Generate sparse vectors for text",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Generate vector for single text
  python main.py -text "Hello world"
  
  # Generate vectors from file
  python main.py -batch -file inputs.jsonl
  
  # Download model
  python main.py -download -model "naver/splade-v3" -source "modelscope"
        """
    )
    
    parser.add_argument("-text", type=str, help="Input text to embed")
    parser.add_argument("-batch", action="store_true", help="Process batch input from file")
    parser.add_argument("-file", type=str, help="Input file path (JSONL format, one {\"text\": \"...\"} per line)")
    parser.add_argument("-download", action="store_true", help="Download model only")
    parser.add_argument("-model", type=str, default="naver/splade-v3", help="Model name/ID (default: naver/splade-v3)")
    parser.add_argument("-source", type=str, default="modelscope", choices=["modelscope", "huggingface"],
                       help="Model source (default: modelscope)")
    parser.add_argument("-cache_dir", type=str, default=".splade_models", help="Model cache directory")
    parser.add_argument("-max_length", type=int, default=256, help="Maximum sequence length")
    parser.add_argument("-vector_dim", type=int, default=30522, help="Vector dimension")
    
    args = parser.parse_args()
    
    # Handle download mode
    if args.download:
        model_path = download_model(args.model, args.source, args.cache_dir)
        result = {
            "success": True,
            "model_path": model_path,
            "message": f"Model {args.model} downloaded successfully"
        }
        print(json.dumps(result))
        return
    
    # Validate input
    if not args.text and not args.batch:
        parser.error("Either -text or -batch must be specified")
    
    # Download model if not exists
    model_path = Path(args.cache_dir) / args.model.replace("/", "_")
    if not model_path.exists() or not any(model_path.iterdir()):
        if args.auto_download if hasattr(args, 'auto_download') else True:
            model_path = download_model(args.model, args.source, args.cache_dir)
        else:
            print(json.dumps({"success": False, "error": f"Model not found at {model_path}. Use -download to download."}))
            sys.exit(1)
    
    # Load model
    model, tokenizer, device = load_model(str(model_path))
    
    # Single text mode
    if args.text:
        vector_dict = generate_splade_vector(args.text, model, tokenizer, device, args.max_length)
        formatted = format_sparse_vector(vector_dict, args.vector_dim)
        
        result = {
            "success": True,
            "text": args.text,
            "vector": formatted,
            "non_zero_count": len(vector_dict)
        }
        print(json.dumps(result))
        return
    
    # Batch mode
    if args.batch and args.file:
        if not os.path.exists(args.file):
            result = {"success": False, "error": f"File not found: {args.file}"}
            print(json.dumps(result))
            sys.exit(1)
        
        results = []
        with open(args.file, 'r', encoding='utf-8') as f:
            for line_num, line in enumerate(f, 1):
                line = line.strip()
                if not line:
                    continue
                
                try:
                    data = json.loads(line)
                    text = data.get("text", "")
                    if not text:
                        continue
                    
                    vector_dict = generate_splade_vector(text, model, tokenizer, device, args.max_length)
                    formatted = format_sparse_vector(vector_dict, args.vector_dim)
                    
                    results.append({
                        "line": line_num,
                        "text": text,
                        "vector": formatted,
                        "non_zero_count": len(vector_dict)
                    })
                except json.JSONDecodeError as e:
                    print(f"Warning: Skipping invalid JSON at line {line_num}: {e}", file=sys.stderr)
                except Exception as e:
                    print(f"Warning: Error processing line {line_num}: {e}", file=sys.stderr)
        
        result = {
            "success": True,
            "total": len(results),
            "results": results
        }
        print(json.dumps(result))
        return


if __name__ == "__main__":
    main()
