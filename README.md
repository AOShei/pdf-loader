# PDF Loader

[![Go Version](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](https://golang.org/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A lightweight, dependency-free Go library for extracting text content from PDF files. Built from scratch without external PDF libraries, implementing core PDF specification parsing for text extraction.

Originally developed for RAG (Retrieval-Augmented Generation) and LLM document processing pipelines where simple, reliable text extraction is needed without the complexity of full-featured PDF libraries.

## Features

### ✅ Implemented

- **Pure Go implementation** - Zero external dependencies, uses only Go standard library
- **Cross-reference table parsing** - Supports both standard xref tables and compressed xref streams
- **Object stream decompression** - Handles modern PDFs with compressed object storage
- **Text extraction** - Full text state machine with proper font metrics and spacing
- **Character mapping** - ToUnicode CMap parsing for accurate character decoding
- **Font encoding support** - PDF /Encoding dictionary parsing with glyph name to Unicode mapping
- **Ligature support** - Proper rendering of text ligatures (fi, fl, ff, ffi, ffl) as multi-character strings
- **Mathematical symbols** - Comprehensive support for Greek letters (α, β, π, Σ, Ω), operators (×, ÷, ≠, ≤, ≥, ∞, ∫, √, ∂), and astronomy symbols (⊙)
- **Superscripts and subscripts** - Unicode rendering of superior/inferior numbers and symbols
- **Hex string parsing** - Correct conversion of PDF hex strings to bytes per PDF 1.7 specification
- **Control character filtering** - Removes non-printable control characters while preserving intentional whitespace
- **Octal escape sequences** - Proper handling of PDF string literals with octal escapes (e.g., `\050` → `(`)
- **Page tree traversal** - Recursive navigation of nested page structures
- **Metadata extraction** - Extracts title, author, creator, producer from document info
- **JSON output** - Structured output with page numbers, dimensions, and character counts
- **FlateDecode support** - Automatic decompression of zlib-compressed streams
- **Encryption support** - Automatic decryption of owner-password-only PDFs (RC4 40/128-bit, AES-128) that are viewable without a password

### ⚠️ Limitations

- **CID fonts** - Limited support for complex Type0 fonts (common in Asian language PDFs)
- **User-password encryption** - No support for PDFs requiring a password to view (only owner-password-only PDFs are supported)
- **AES-256 encryption** - Only RC4 and AES-128 encryption are supported (AES-256 from PDF 1.7 Extension Level 3 not yet implemented)
- **Images** - Text-only extraction, images are ignored
- **Advanced filters** - Only FlateDecode implemented (no LZW, JPEG, ASCII85, etc.)
- **Complex layouts** - May struggle with multi-column text, tables, or right-to-left text

## Installation

```bash
go get github.com/AOShei/pdf-loader
```

## Usage

### Command Line

```bash
# Run directly
go run cmd/main.go path/to/document.pdf

# Or build and run
go build -o pdf-loader cmd/main.go
./pdf-loader path/to/document.pdf
```

Output is JSON to stdout:

```json
{
  "metadata": {
    "title": "Lorem Ipsum Document",
    "author": "John Doe",
    "creator": "LaTeX with hyperref",
    "producer": "pdfTeX-1.40.21",
    "encrypted": false
  },
  "pages": [
    {
      "page_number": 1,
      "content": "Lorem ipsum dolor sit amet, consectetur adipiscing elit...",
      "char_count": 1847,
      "width": 612.0,
      "height": 792.0
    },
    {
      "page_number": 2,
      "content": "Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua...",
      "char_count": 2156,
      "width": 612.0,
      "height": 792.0
    }
  ]
}
```

### Library API

```go
package main

import (
    "encoding/json"
    "fmt"
    "log"
    
    "github.com/AOShei/pdf-loader/pkg/loader"
)

func main() {
    // Load PDF and extract text
    doc, err := loader.LoadPDF("document.pdf")
    if err != nil {
        log.Fatal(err)
    }
    
    // Access metadata
    fmt.Printf("Title: %s\n", doc.Metadata.Title)
    fmt.Printf("Author: %s\n", doc.Metadata.Author)
    
    // Iterate through pages
    for _, page := range doc.Pages {
        fmt.Printf("\n--- Page %d (%d characters) ---\n", 
            page.PageNumber, page.CharCount)
        fmt.Println(page.Content)
    }
    
    // Convert to JSON
    jsonData, _ := json.MarshalIndent(doc, "", "  ")
    fmt.Println(string(jsonData))
}
```

## Architecture

The library implements a multi-stage parsing pipeline:

```
PDF File → Reader (xref resolution) → Page Dictionary → Extractor (state machines) → Text → Document (JSON)
```

**Key Components:**

- **`pkg/pdf/lexer.go`** - Tokenizes PDF objects (dictionaries, arrays, streams, etc.)
  - Handles PDF string literals with escape sequences (`\n`, `\r`, `\nnn` octal escapes)
  - Converts hex strings (`<48656C6C6F>`) to bytes per PDF 1.7 spec
- **`pkg/pdf/reader.go`** - Coordinates parsing, resolves indirect object references
  - Initializes encryption handler and decrypts objects automatically
- **`pkg/pdf/encrypt.go`** - PDF encryption/decryption support
  - Implements Algorithm 2 (file encryption key generation) and Algorithm 1 (per-object key derivation) from PDF spec
  - Supports RC4 (40-bit, 128-bit) and AES-128 CBC mode decryption
  - Automatically decrypts strings and streams before processing
- **`pkg/pdf/xref.go`** - Handles cross-reference tables and object lookup
- **`pkg/pdf/extractor.go`** - Text extraction with graphics/text state machines
  - Parses `/Encoding` dictionaries with `/Differences` arrays
  - Maps 250+ glyph names to Unicode (ligatures, math symbols, Greek letters, astronomy symbols)
  - Supports three decoding paths: ToUnicode CMap, /Encoding dictionary, or direct byte conversion
  - Filters non-printable control characters while preserving intentional whitespace
- **`pkg/pdf/cmap.go`** - Character mapping for font encoding (ToUnicode CMaps)
- **`pkg/pdf/content.go`** - Content stream operator parsing
- **`pkg/loader/loader.go`** - High-level API orchestrating the pipeline
- **`pkg/model/types.go`** - Output data structures (`Document`, `Page`, `Metadata`)

The extractor maintains two state machines:
1. **Graphics State** - Current Transformation Matrix (CTM), saved/restored with `q`/`Q` operators
2. **Text State** - Font metrics, Text Matrix (TM), positioning, spacing, and scale

**Text Decoding Strategy:**
1. If font has ToUnicode CMap → use CMap for character code to Unicode mapping
2. Else if font has /Encoding dictionary → use glyph name mapping (handles embedded fonts)
3. Else → direct byte-to-character conversion (assumes standard ASCII)

## Dependencies

**Zero external dependencies** - built entirely with Go standard library:

- `bufio` - Buffered I/O for efficient parsing
- `compress/zlib` - FlateDecode stream decompression
- `crypto/md5` - MD5 hashing for encryption key generation
- `crypto/rc4` - RC4 decryption for encrypted PDFs
- `crypto/aes` - AES decryption for encrypted PDFs
- `crypto/cipher` - Cipher block chaining (CBC) mode for AES
- `encoding/json` - JSON output formatting
- `unicode/utf16` - UTF-16BE decoding for CMaps

## Known Issues

1. **Type assertion fragility** - Some type conversions lack error checks and may panic on malformed PDFs
2. **Spacing heuristics** - Text spacing detection uses heuristics that may fail on complex layouts
3. **No caching** - Objects may be re-parsed multiple times (performance trade-off for simplicity)
4. **Partial spec compliance** - Implements subset of PDF 1.7 specification sufficient for text extraction
5. **Subscript/superscript positioning** - Superscripts and subscripts are rendered as Unicode characters but positioning information is not preserved (e.g., "10^12" may appear as "10¹²" rather than showing the layout)

## Roadmap

Future development focuses on expanding extraction capabilities for document processing pipelines:

- AES-256 encryption support (PDF 1.7 Extension Level 3)
- Additional stream filter support (LZW, ASCII85)
- Improved CID font handling for better Unicode coverage
- Table detection and structured content extraction
- Enhanced spacing/layout preservation for multi-column documents
- Performance optimizations (object caching, parallel page processing)

## License

MIT License - see [LICENSE](LICENSE) file for details.

Copyright (c) 2025 Andrew O'Shei
