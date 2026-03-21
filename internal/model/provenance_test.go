package model

import (
	"bytes"
	"testing"
)

func TestProvenance_WriteFooter(t *testing.T) {
	tests := []struct {
		name string
		prov Provenance
		want string
	}{
		{
			name: "with command",
			prov: Provenance{Command: "gh velocity flow lead-time --since 30d"},
			want: "\n— gh velocity flow lead-time --since 30d\n",
		},
		{
			name: "empty command",
			prov: Provenance{},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.prov.WriteFooter(&buf)
			if got := buf.String(); got != tt.want {
				t.Errorf("WriteFooter() = %q, want %q", got, tt.want)
			}
		})
	}
}
