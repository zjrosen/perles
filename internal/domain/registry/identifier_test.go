package registry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseIdentifier(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantNamespace string
		wantKey       string
		wantVersion   string
		wantChainKey  string
		wantErr       error
	}{
		{
			name:          "full identifier",
			input:         "spec-workflow::planning-standard::v1::research",
			wantNamespace: "spec-workflow",
			wantKey:       "planning-standard",
			wantVersion:   "v1",
			wantChainKey:  "research",
		},
		{
			name:          "identifier with hyphenated key",
			input:         "spec-workflow::planning-simple::v1::plan",
			wantNamespace: "spec-workflow",
			wantKey:       "planning-simple",
			wantVersion:   "v1",
			wantChainKey:  "plan",
		},
		{
			name:          "chain key with hyphen",
			input:         "spec-workflow::planning-simple::v1::research-propose",
			wantNamespace: "spec-workflow",
			wantKey:       "planning-simple",
			wantVersion:   "v1",
			wantChainKey:  "research-propose",
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: ErrInvalidIdentifier,
		},
		{
			name:    "only namespace (1 part)",
			input:   "spec-workflow",
			wantErr: ErrInvalidIdentifier,
		},
		{
			name:    "missing version and chain key (2 parts)",
			input:   "spec-workflow::planning-standard",
			wantErr: ErrInvalidIdentifier,
		},
		{
			name:    "missing chain key (3 parts)",
			input:   "spec-workflow::planning-standard::v1",
			wantErr: ErrInvalidIdentifier,
		},
		{
			name:    "too many parts (5 parts)",
			input:   "spec::workflow::planning-standard::v1::research",
			wantErr: ErrInvalidIdentifier,
		},
		{
			name:    "no separators at all",
			input:   "invalid",
			wantErr: ErrInvalidIdentifier,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseIdentifier(tt.input)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.Nil(t, result)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, tt.wantNamespace, result.Namespace)
			require.Equal(t, tt.wantKey, result.Key)
			require.Equal(t, tt.wantVersion, result.Version)
			require.Equal(t, tt.wantChainKey, result.ChainKey)
		})
	}
}

func TestBuildIdentifier(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		key       string
		version   string
		chainKey  string
		want      string
	}{
		{
			name:      "standard workflow",
			namespace: "spec-workflow",
			key:       "planning-standard",
			version:   "v1",
			chainKey:  "research",
			want:      "spec-workflow::planning-standard::v1::research",
		},
		{
			name:      "different namespace",
			namespace: "spec-template",
			key:       "standard",
			version:   "v1",
			chainKey:  "create",
			want:      "spec-template::standard::v1::create",
		},
		{
			name:      "hyphenated chain key",
			namespace: "spec-workflow",
			key:       "planning-simple",
			version:   "v1",
			chainKey:  "research-propose",
			want:      "spec-workflow::planning-simple::v1::research-propose",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildIdentifier(tt.namespace, tt.key, tt.version, tt.chainKey)
			require.Equal(t, tt.want, result)
		})
	}
}

func TestParseIdentifier_RoundTrip(t *testing.T) {
	// Test that we can build an identifier and parse it back
	identifiers := []struct {
		namespace string
		key       string
		version   string
		chainKey  string
	}{
		{"spec-workflow", "planning-standard", "v1", "research"},
		{"spec-workflow", "planning-standard", "v1", "propose"},
		{"spec-workflow", "planning-standard", "v1", "plan"},
		{"spec-workflow", "planning-simple", "v1", "research-propose"},
	}

	for _, id := range identifiers {
		built := BuildIdentifier(id.namespace, id.key, id.version, id.chainKey)
		t.Run(built, func(t *testing.T) {
			parsed, err := ParseIdentifier(built)
			require.NoError(t, err)
			require.Equal(t, id.namespace, parsed.Namespace)
			require.Equal(t, id.key, parsed.Key)
			require.Equal(t, id.version, parsed.Version)
			require.Equal(t, id.chainKey, parsed.ChainKey)
		})
	}
}
