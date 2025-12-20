package pdf

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rc4"
	"errors"
	"fmt"
)

// EncryptDict represents the PDF encryption dictionary
type EncryptDict struct {
	Filter          string // Should be "/Standard"
	V               int    // Version: 1, 2, 4
	R               int    // Revision: 2, 3, 4
	O               []byte // Owner password hash (48 bytes)
	U               []byte // User password hash (48 bytes)
	P               int32  // Permission flags
	Length          int    // Key length in bits (40, 128)
	EncryptMetadata bool   // Usually true
}

// EncryptionHandler handles PDF encryption/decryption
type EncryptionHandler struct {
	Dict       *EncryptDict
	FileID     []byte // From trailer /ID
	EncryptKey []byte // Computed encryption key
	V          int    // Algorithm version
	R          int    // Standard security handler revision
}

// PDF standard padding string (32 bytes) - from PDF spec
var paddingString = []byte{
	0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41,
	0x64, 0x00, 0x4E, 0x56, 0xFF, 0xFA, 0x01, 0x08,
	0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80,
	0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
}

// ParseEncryptDict extracts encryption dictionary from a PDF object
func ParseEncryptDict(obj Object, reader *Reader) (*EncryptDict, error) {
	dict, ok := obj.(DictionaryObject)
	if !ok {
		return nil, errors.New("encryption object is not a dictionary")
	}

	encDict := &EncryptDict{
		EncryptMetadata: true, // Default value
	}

	// Extract Filter (should be "/Standard")
	if filter, ok := dict["/Filter"].(NameObject); ok {
		encDict.Filter = string(filter)
	} else {
		return nil, errors.New("missing or invalid /Filter in encryption dictionary")
	}

	if encDict.Filter != "/Standard" {
		return nil, fmt.Errorf("unsupported encryption filter: %s", encDict.Filter)
	}

	// Extract V (version)
	if v, ok := dict["/V"].(NumberObject); ok {
		encDict.V = int(v)
	} else {
		return nil, errors.New("missing or invalid /V in encryption dictionary")
	}

	// Extract R (revision)
	if r, ok := dict["/R"].(NumberObject); ok {
		encDict.R = int(r)
	} else {
		return nil, errors.New("missing or invalid /R in encryption dictionary")
	}

	// Extract O (owner password hash)
	oObj := reader.Resolve(dict["/O"])
	switch o := oObj.(type) {
	case StringObject:
		encDict.O = []byte(o)
	case HexStringObject:
		encDict.O = []byte(o)
	default:
		return nil, errors.New("missing or invalid /O in encryption dictionary")
	}

	// Extract U (user password hash)
	uObj := reader.Resolve(dict["/U"])
	switch u := uObj.(type) {
	case StringObject:
		encDict.U = []byte(u)
	case HexStringObject:
		encDict.U = []byte(u)
	default:
		return nil, errors.New("missing or invalid /U in encryption dictionary")
	}

	// Extract P (permissions)
	if p, ok := dict["/P"].(NumberObject); ok {
		encDict.P = int32(p)
	} else {
		return nil, errors.New("missing or invalid /P in encryption dictionary")
	}

	// Extract Length (key length in bits)
	if length, ok := dict["/Length"].(NumberObject); ok {
		encDict.Length = int(length)
	} else {
		// Default lengths based on revision
		if encDict.R == 2 {
			encDict.Length = 40
		} else {
			encDict.Length = 128
		}
	}

	// Extract EncryptMetadata (optional, default true)
	if em, ok := dict["/EncryptMetadata"].(BooleanObject); ok {
		encDict.EncryptMetadata = bool(em)
	}

	return encDict, nil
}

// NewEncryptionHandler creates a new encryption handler with empty password
func NewEncryptionHandler(encDict *EncryptDict, fileID []byte) (*EncryptionHandler, error) {
	if encDict == nil {
		return nil, errors.New("encryption dictionary is nil")
	}

	handler := &EncryptionHandler{
		Dict:   encDict,
		FileID: fileID,
		V:      encDict.V,
		R:      encDict.R,
	}

	// Compute encryption key with empty password (for owner-password-only PDFs)
	handler.EncryptKey = handler.computeEncryptionKey([]byte{})

	return handler, nil
}

// padPassword pads or truncates password to 32 bytes using PDF standard padding
func padPassword(password []byte) []byte {
	padded := make([]byte, 32)
	copy(padded, password)
	if len(password) < 32 {
		copy(padded[len(password):], paddingString)
	}
	return padded
}

// encodePermissions converts permission flags to byte array (little-endian)
func encodePermissions(p int32) []byte {
	return []byte{
		byte(p),
		byte(p >> 8),
		byte(p >> 16),
		byte(p >> 24),
	}
}

// computeEncryptionKey implements Algorithm 2 from PDF spec
// Computes the file encryption key from password
func (h *EncryptionHandler) computeEncryptionKey(password []byte) []byte {
	// 1. Pad password to 32 bytes
	padded := padPassword(password)

	// 2. Initialize MD5 hash
	hash := md5.New()

	// 3. Hash components in order:
	//    a) Padded password
	hash.Write(padded)

	//    b) O entry from encryption dictionary
	hash.Write(h.Dict.O)

	//    c) P entry (permissions) as 4-byte little-endian
	hash.Write(encodePermissions(h.Dict.P))

	//    d) File ID from trailer
	hash.Write(h.FileID)

	// 4. If R >= 4 and EncryptMetadata is false, hash 0xFFFFFFFF
	if h.R >= 4 && !h.Dict.EncryptMetadata {
		hash.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	}

	// 5. Get hash digest
	digest := hash.Sum(nil)

	// 6. If R >= 3, do 50 additional MD5 iterations
	if h.R >= 3 {
		keyLen := h.Dict.Length / 8
		for i := 0; i < 50; i++ {
			hash := md5.New()
			hash.Write(digest[:keyLen])
			digest = hash.Sum(nil)
		}
	}

	// 7. Return first n bytes (n = Length/8)
	keyLen := h.Dict.Length / 8
	return digest[:keyLen]
}

// computeObjectKey implements Algorithm 1 from PDF spec
// Derives per-object encryption key from file encryption key
func (h *EncryptionHandler) computeObjectKey(objNum, genNum int) []byte {
	// Start with file encryption key
	keyLen := len(h.EncryptKey)
	key := make([]byte, keyLen+5)
	copy(key, h.EncryptKey)

	// Add object number (3 bytes, little-endian)
	key[keyLen] = byte(objNum)
	key[keyLen+1] = byte(objNum >> 8)
	key[keyLen+2] = byte(objNum >> 16)

	// Add generation number (2 bytes, little-endian)
	key[keyLen+3] = byte(genNum)
	key[keyLen+4] = byte(genNum >> 8)

	// If AES (V >= 4), add salt "sAlT"
	if h.V >= 4 {
		key = append(key, 0x73, 0x41, 0x6C, 0x54)
	}

	// MD5 hash the extended key
	hash := md5.Sum(key)

	// Return first min(len(EncryptKey) + 5, 16) bytes
	n := keyLen + 5
	if n > 16 {
		n = 16
	}
	return hash[:n]
}

// decryptRC4 decrypts data using RC4 algorithm
func (h *EncryptionHandler) decryptRC4(data []byte, objNum, genNum int) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	key := h.computeObjectKey(objNum, genNum)

	cipher, err := rc4.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create RC4 cipher: %w", err)
	}

	decrypted := make([]byte, len(data))
	cipher.XORKeyStream(decrypted, data)

	return decrypted, nil
}

// removePadding removes PKCS7 padding from decrypted data
func removePadding(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	paddingLen := int(data[len(data)-1])

	// Validate padding length
	if paddingLen == 0 || paddingLen > 16 || paddingLen > len(data) {
		// No padding or invalid padding
		return data, nil
	}

	// Verify padding bytes are all the same
	for i := len(data) - paddingLen; i < len(data); i++ {
		if data[i] != byte(paddingLen) {
			// Invalid padding, return as-is
			return data, nil
		}
	}

	return data[:len(data)-paddingLen], nil
}

// decryptAES decrypts data using AES-128 in CBC mode
func (h *EncryptionHandler) decryptAES(data []byte, objNum, genNum int) ([]byte, error) {
	if len(data) < 16 {
		return nil, fmt.Errorf("encrypted data too short for AES (need at least 16 bytes for IV, got %d)", len(data))
	}

	key := h.computeObjectKey(objNum, genNum)

	// First 16 bytes are IV (initialization vector)
	iv := data[:16]
	ciphertext := data[16:]

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Decrypt using CBC mode
	mode := cipher.NewCBCDecrypter(block, iv)

	decrypted := make([]byte, len(ciphertext))
	mode.CryptBlocks(decrypted, ciphertext)

	// Remove PKCS7 padding
	return removePadding(decrypted)
}

// Decrypt decrypts data for a specific object using appropriate algorithm
func (h *EncryptionHandler) Decrypt(data []byte, objNum, genNum int) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	switch h.V {
	case 1, 2:
		// RC4 encryption (40-bit or 128-bit)
		return h.decryptRC4(data, objNum, genNum)
	case 4:
		// AES-128 encryption
		return h.decryptAES(data, objNum, genNum)
	default:
		return nil, fmt.Errorf("unsupported encryption version: %d (only V1, V2, V4 are supported)", h.V)
	}
}
