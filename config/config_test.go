package config

import (
	"testing"
)

func TestParsePSKs(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantPsks     []string
		wantPskMap   map[string]string
		wantFlatMode bool
	}{
		{
			name:         "empty string",
			input:        "",
			wantPsks:     nil,
			wantPskMap:   map[string]string{},
			wantFlatMode: false,
		},
		{
			name:         "single flat PSK",
			input:        "my-secret-psk",
			wantPsks:     []string{"my-secret-psk"},
			wantFlatMode: true,
		},
		{
			name:         "multiple flat PSKs",
			input:        "psk1,psk2,psk3",
			wantPsks:     []string{"psk1", "psk2", "psk3"},
			wantFlatMode: true,
		},
		{
			name:         "flat PSKs with whitespace",
			input:        " psk1 , psk2 ",
			wantPsks:     []string{"psk1", "psk2"},
			wantFlatMode: true,
		},
		{
			name:  "JSON app-bound PSKs",
			input: `{"subscriptions":"psk-subs","urn:redhat:application:inventory":"psk-inv"}`,
			wantPskMap: map[string]string{
				"subscriptions":                    "psk-subs",
				"urn:redhat:application:inventory": "psk-inv",
			},
			wantFlatMode: false,
		},
		{
			name:         "invalid JSON falls back to flat",
			input:        `{invalid-json`,
			wantPsks:     []string{"{invalid-json"},
			wantFlatMode: true,
		},
		{
			name:         "empty JSON object falls back to flat",
			input:        `{}`,
			wantPsks:     []string{"{}"},
			wantFlatMode: true,
		},
		{
			name:         "trailing commas ignored",
			input:        "psk1,,psk2,",
			wantPsks:     []string{"psk1", "psk2"},
			wantFlatMode: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			psks, pskMap := parsePSKs(tt.input)

			if tt.wantFlatMode {
				if len(pskMap) != 0 {
					t.Errorf("expected empty pskMap in flat mode, got %v", pskMap)
				}
				if len(psks) != len(tt.wantPsks) {
					t.Fatalf("expected %d psks, got %d: %v", len(tt.wantPsks), len(psks), psks)
				}
				for i, want := range tt.wantPsks {
					if psks[i] != want {
						t.Errorf("psks[%d] = %q, want %q", i, psks[i], want)
					}
				}
			} else {
				if psks != nil {
					t.Errorf("expected nil psks in map mode, got %v", psks)
				}
				if len(pskMap) != len(tt.wantPskMap) {
					t.Fatalf("expected %d pskMap entries, got %d: %v", len(tt.wantPskMap), len(pskMap), pskMap)
				}
				for k, want := range tt.wantPskMap {
					got, ok := pskMap[k]
					if !ok {
						t.Errorf("pskMap missing key %q", k)
					} else if got != want {
						t.Errorf("pskMap[%q] = %q, want %q", k, got, want)
					}
				}
			}
		})
	}
}
