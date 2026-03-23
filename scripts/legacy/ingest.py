#!/usr/bin/env python3
"""
Ingest writing samples into Qdrant for the ghostwriter RAG system.

Reads PR review comments from corpus/reviews.jsonl, embeds them using
FastEmbed (local, no API key needed), and stores them in a local Qdrant
instance.

Usage:
    python3 ingest.py [--qdrant-url URL] [--collection NAME] [--reset]
                      [--no-normalize-dashes]

Options:
    --qdrant-url          Qdrant server URL (default: from .env or http://127.0.0.1:6333)
    --collection          Collection name (default: from .env or writing-samples)
    --reset               Delete and recreate the collection before ingesting
    --no-normalize-dashes Skip dash-to-comma normalization in documents
"""

import argparse
import json
import os
import re
import sys
import uuid
from pathlib import Path

try:
    from qdrant_client import QdrantClient, models
    from fastembed import TextEmbedding
except ImportError:
    print("Missing dependencies. Install with:")
    print("  pip3 install -r requirements.txt")
    sys.exit(1)

# Try to load .env file
try:
    from dotenv import load_dotenv

    load_dotenv(Path(__file__).parent.parent / ".env")
except ImportError:
    pass  # python-dotenv is optional


def load_corpus(corpus_path: Path) -> list[dict]:
    """Load writing samples from JSONL file."""
    if not corpus_path.exists():
        print(f"Corpus file not found: {corpus_path}")
        print("Run collect-github-reviews.sh first to gather writing samples.")
        sys.exit(1)

    samples = []
    with open(corpus_path) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                sample = json.loads(line)
                # Filter out very short comments (less than 50 chars)
                if len(sample.get("body", "")) >= 50:
                    samples.append(sample)
            except json.JSONDecodeError:
                continue

    return samples


def normalize_dashes(text: str) -> str:
    """Replace em dashes, en dashes, and double hyphens used as sentence
    connectors with commas. Preserves hyphens in hyphenated words (e.g.,
    'cloud-agnostic'), code blocks, and inline code spans."""
    # Protect code blocks and inline code from replacement
    protected = []
    counter = 0

    def protect(match):
        nonlocal counter
        placeholder = f"\x00PROTECTED{counter}\x00"
        protected.append((placeholder, match.group(0)))
        counter += 1
        return placeholder

    # Protect fenced code blocks first, then inline code
    result = re.sub(r"```[\s\S]*?```", protect, text)
    result = re.sub(r"`[^`]+`", protect, result)

    # Replace em dash, en dash, and double hyphen used as sentence connectors
    result = re.sub(r"\s*—\s*", ", ", result)
    result = re.sub(r"\s*–\s*", ", ", result)
    result = re.sub(r"\s+--\s+", ", ", result)

    # Restore protected code blocks and inline code
    for placeholder, original in protected:
        result = result.replace(placeholder, original)

    return result


def build_document(sample: dict, do_normalize_dashes: bool = True) -> str:
    """Build a searchable document from a writing sample."""
    parts = []

    if sample.get("type") == "review_summary":
        parts.append(f"[PR Review Summary] {sample.get('pr_title', '')}")
    elif sample.get("type") == "inline_comment":
        parts.append(f"[Inline Code Comment] {sample.get('pr_title', '')}")
        if sample.get("file_path"):
            parts.append(f"File: {sample['file_path']}")

    body = sample["body"]
    if do_normalize_dashes:
        body = normalize_dashes(body)
    parts.append(body)

    return "\n".join(parts)


def main():
    parser = argparse.ArgumentParser(description="Ingest writing samples into Qdrant")
    parser.add_argument(
        "--qdrant-url",
        default=os.environ.get("QDRANT_URL", "http://127.0.0.1:6333"),
        help="Qdrant server URL (default: from .env or http://127.0.0.1:6333)",
    )
    parser.add_argument(
        "--collection",
        default=os.environ.get("COLLECTION_NAME", "writing-samples"),
        help="Collection name (default: from .env or writing-samples)",
    )
    parser.add_argument(
        "--reset",
        action="store_true",
        help="Delete and recreate the collection before ingesting",
    )
    parser.add_argument(
        "--no-normalize-dashes",
        action="store_true",
        help="Skip dash-to-comma normalization (if you naturally use dashes)",
    )
    args = parser.parse_args()

    do_normalize = not args.no_normalize_dashes

    # Load corpus
    script_dir = Path(__file__).parent
    corpus_path = script_dir / "corpus" / "reviews.jsonl"
    samples = load_corpus(corpus_path)
    print(f"Loaded {len(samples)} writing samples from {corpus_path}")

    if not samples:
        print("No samples to ingest.")
        sys.exit(0)

    if do_normalize:
        print("Dash normalization: enabled (use --no-normalize-dashes to disable)")
    else:
        print("Dash normalization: disabled")

    # Initialize embedding model (downloads on first run, ~130MB)
    print("Loading embedding model (BAAI/bge-small-en-v1.5)...")
    embedding_model = TextEmbedding("BAAI/bge-small-en-v1.5")

    # Build documents for embedding
    documents = [build_document(s, do_normalize) for s in samples]
    print(f"Embedding {len(documents)} documents...")

    # Generate embeddings
    embeddings = list(embedding_model.embed(documents))
    vector_size = len(embeddings[0])
    print(f"Generated {len(embeddings)} embeddings (dimension: {vector_size})")

    # Connect to Qdrant
    print(f"Connecting to Qdrant at {args.qdrant_url}...")
    client = QdrantClient(url=args.qdrant_url)

    # Create or reset collection
    collections = [c.name for c in client.get_collections().collections]
    if args.reset and args.collection in collections:
        print(f"Deleting existing collection '{args.collection}'...")
        client.delete_collection(args.collection)
        collections.remove(args.collection)

    if args.collection not in collections:
        print(f"Creating collection '{args.collection}'...")
        client.create_collection(
            collection_name=args.collection,
            vectors_config=models.VectorParams(
                size=vector_size,
                distance=models.Distance.COSINE,
            ),
        )

    # Build points with metadata
    points = []
    for i, (sample, embedding) in enumerate(zip(samples, embeddings)):
        point_id = str(uuid.uuid5(uuid.NAMESPACE_DNS, sample["body"][:200]))
        payload = {
            "document": documents[i],
            "type": sample.get("type", "unknown"),
            "repo": sample.get("repo", ""),
            "pr_number": sample.get("pr_number", 0),
            "pr_title": sample.get("pr_title", ""),
            "file_path": sample.get("file_path", ""),
            "created_at": sample.get("created_at", ""),
            "body_length": len(sample.get("body", "")),
        }
        points.append(
            models.PointStruct(
                id=point_id,
                vector=embedding.tolist(),
                payload=payload,
            )
        )

    # Upsert in batches
    batch_size = 64
    for i in range(0, len(points), batch_size):
        batch = points[i : i + batch_size]
        client.upsert(collection_name=args.collection, points=batch)
        print(f"  Upserted {min(i + batch_size, len(points))}/{len(points)} points")

    # Verify
    collection_info = client.get_collection(args.collection)
    print(
        f"\nDone! Collection '{args.collection}' now has "
        f"{collection_info.points_count} points."
    )
    print(f"Vector dimension: {vector_size}")
    print(f"Distance metric: cosine")

    # Quick test search
    print("\nRunning test search for 'code review suggestion'...")
    test_embedding = list(
        embedding_model.embed(["code review suggestion with alternative approach"])
    )[0]
    results = client.query_points(
        collection_name=args.collection,
        query=test_embedding.tolist(),
        limit=2,
    )
    for hit in results.points:
        preview = hit.payload.get("document", "")[:120].replace("\n", " ")
        print(f"  Score: {hit.score:.3f} | {preview}...")


if __name__ == "__main__":
    main()
