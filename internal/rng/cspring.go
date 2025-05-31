// internal/rng/csprng.go
package rng

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/binary"
    "fmt"
    "io"
    "sync"
)

// CSPRNG uses AES-CTR under the hood. It is seeded once from crypto/rand.
type CSPRNG struct {
    mu       sync.Mutex
    block    cipher.Block
    counter  [16]byte   // 128-bit counter for AES-CTR
    stream   cipher.Stream
}

// NewCSPRNG initializes an AES-CTR generator seeded from crypto/rand.
func NewCSPRNG() (*CSPRNG, error) {
    // 1) Generate a 256-bit AES key from crypto/rand
    key := make([]byte, 32)
    if _, err := io.ReadFull(rand.Reader, key); err != nil {
        return nil, fmt.Errorf("rng: failed to get seed from crypto/rand: %w", err)
    }

    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, fmt.Errorf("rng: aes.NewCipher failed: %w", err)
    }

    // 2) Initialize counter to a random IV (128 bits)
    var iv [16]byte
    if _, err := io.ReadFull(rand.Reader, iv[:]); err != nil {
        return nil, fmt.Errorf("rng: failed to get IV from crypto/rand: %w", err)
    }

    stream := cipher.NewCTR(block, iv[:])

    return &CSPRNG{
        block:   block,
        counter: iv,
        stream:  stream,
    }, nil
}

// Read fills buf with cryptographically secure random bytes (AES-CTR output).
func (c *CSPRNG) Read(buf []byte) (int, error) {
    c.mu.Lock()
    defer c.mu.Unlock()
    // Note: Under the hood, AES-CTR XORs a keystream block into buf.
    c.stream.XORKeyStream(buf, buf) // buf is initially zero, so it becomes keystream
    return len(buf), nil
}

// Uint32 returns a single 32-bit random word.
func (c *CSPRNG) Uint32() (uint32, error) {
    var b [4]byte
    if _, err := c.Read(b[:]); err != nil {
        return 0, err
    }
    return binary.BigEndian.Uint32(b[:]), nil
}
