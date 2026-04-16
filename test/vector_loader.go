// Package test provides helpers for loading shared test vector JSON files.
package test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

// VectorFile represents the top-level structure of a test vector JSON file.
type VectorFile struct {
	Description string                 `json:"description"`
	Note        string                 `json:"note,omitempty"`
	Keys        map[string]KeyPair     `json:"keys,omitempty"`
	Vectors     []json.RawMessage      `json:"vectors,omitempty"`
	Raw         map[string]interface{} `json:"-"`
}

// KeyPair holds a test key pair.
type KeyPair struct {
	PrivKey string `json:"privkey,omitempty"`
	PubKey  string `json:"pubkey"`
}

// vectorDir returns the absolute path to the shared test-vectors directory.
// Vectors are sourced from the nostr-mail-nip submodule at external/nostr-mail-nip.
func vectorDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "external", "nostr-mail-nip", "test-vectors")
}

// LoadVectors reads a test vector JSON file from the shared test-vectors directory
// and returns the raw parsed map. This is useful for flexible access to vector data.
func LoadVectors(filename string) (map[string]interface{}, error) {
	data, err := os.ReadFile(filepath.Join(vectorDir(), filename))
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	return result, err
}

// LoadVectorFile reads and parses a test vector file into a VectorFile struct.
func LoadVectorFile(filename string) (*VectorFile, error) {
	data, err := os.ReadFile(filepath.Join(vectorDir(), filename))
	if err != nil {
		return nil, err
	}
	var vf VectorFile
	if err := json.Unmarshal(data, &vf); err != nil {
		return nil, err
	}
	// Also store the raw map for flexible access
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)
	vf.Raw = raw
	return &vf, nil
}

// LoadRawJSON reads a test vector file and returns the raw bytes.
func LoadRawJSON(filename string) ([]byte, error) {
	return os.ReadFile(filepath.Join(vectorDir(), filename))
}

// VectorDir returns the path to the test vectors directory for direct use.
func VectorDir() string {
	return vectorDir()
}
