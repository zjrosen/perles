package registry

import (
	"errors"
	"fmt"
	"strings"
)

// Identifier errors
var (
	ErrInvalidIdentifier = errors.New("invalid identifier format")
)

// IdentifierParts holds the parsed components of an identifier
type IdentifierParts struct {
	Namespace string
	Key       string
	Version   string
	ChainKey  string
}

// ParseIdentifier parses a double-colon-separated identifier into components.
// Format: {namespace}::{key}::{version}::{chain-key}
// Example: spec-workflow::planning-standard::v1::research
//
// All components use "::" as separator. Namespace should not contain "::".
func ParseIdentifier(id string) (*IdentifierParts, error) {
	if id == "" {
		return nil, ErrInvalidIdentifier
	}

	// Split on "::" separator
	parts := strings.Split(id, "::")

	// Format: {namespace}::{key}::{version}::{chain-key}
	// We require exactly 4 parts
	if len(parts) != 4 {
		return nil, ErrInvalidIdentifier
	}

	namespace := parts[0]
	key := parts[1]
	version := parts[2]
	chainKey := parts[3]

	if namespace == "" || key == "" || version == "" || chainKey == "" {
		return nil, ErrInvalidIdentifier
	}

	return &IdentifierParts{
		Namespace: namespace,
		Key:       key,
		Version:   version,
		ChainKey:  chainKey,
	}, nil
}

// BuildIdentifier constructs a valid identifier string from components
func BuildIdentifier(namespace, key, version, chainKey string) string {
	return fmt.Sprintf("%s::%s::%s::%s", namespace, key, version, chainKey)
}
