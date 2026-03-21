package effort

import (
	"testing"

	"github.com/dvhthomas/gh-velocity/internal/config"
)

func ptr(f float64) *float64 { return &f }

func TestCountEvaluator(t *testing.T) {
	e := CountEvaluator{}
	items := []Item{
		{Title: "anything"},
		{Labels: []string{"bug"}},
		{},
	}
	for i, item := range items {
		v, ok := e.Evaluate(item)
		if !ok || v != 1 {
			t.Errorf("CountEvaluator.Evaluate(item[%d]) = (%.0f, %v), want (1, true)", i, v, ok)
		}
	}
}

func TestAttributeEvaluator(t *testing.T) {
	cfg := config.EffortConfig{
		Strategy: "attribute",
		Attribute: []config.EffortMatcher{
			{Query: "label:size/S", Value: 2},
			{Query: "label:size/M", Value: 3},
			{Query: "label:size/L", Value: 5},
			{Query: "label:chore", Value: 0},
		},
	}

	e, err := NewEvaluator(cfg)
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}

	tests := []struct {
		name    string
		item    Item
		wantVal float64
		wantOK  bool
	}{
		{
			name:    "matches first",
			item:    Item{Labels: []string{"size/S"}},
			wantVal: 2, wantOK: true,
		},
		{
			name:    "matches second",
			item:    Item{Labels: []string{"size/M", "bug"}},
			wantVal: 3, wantOK: true,
		},
		{
			name:    "first match wins with overlapping labels",
			item:    Item{Labels: []string{"size/S", "size/L"}},
			wantVal: 2, wantOK: true,
		},
		{
			name:    "no match = not assessed",
			item:    Item{Labels: []string{"bug"}},
			wantVal: 0, wantOK: false,
		},
		{
			name:    "value zero is valid",
			item:    Item{Labels: []string{"chore"}},
			wantVal: 0, wantOK: true,
		},
		{
			name:    "empty labels = not assessed",
			item:    Item{},
			wantVal: 0, wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok := e.Evaluate(tt.item)
			if v != tt.wantVal || ok != tt.wantOK {
				t.Errorf("Evaluate() = (%.0f, %v), want (%.0f, %v)", v, ok, tt.wantVal, tt.wantOK)
			}
		})
	}
}

func TestNumericEvaluator(t *testing.T) {
	e := NumericEvaluator{}

	tests := []struct {
		name    string
		item    Item
		wantVal float64
		wantOK  bool
	}{
		{
			name:    "nil = not assessed",
			item:    Item{Effort: nil},
			wantVal: 0, wantOK: false,
		},
		{
			name:    "zero is valid",
			item:    Item{Effort: ptr(0)},
			wantVal: 0, wantOK: true,
		},
		{
			name:    "positive value",
			item:    Item{Effort: ptr(5)},
			wantVal: 5, wantOK: true,
		},
		{
			name:    "fractional value",
			item:    Item{Effort: ptr(2.5)},
			wantVal: 2.5, wantOK: true,
		},
		{
			name:    "negative = not assessed",
			item:    Item{Effort: ptr(-3)},
			wantVal: 0, wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok := e.Evaluate(tt.item)
			if v != tt.wantVal || ok != tt.wantOK {
				t.Errorf("Evaluate() = (%.1f, %v), want (%.1f, %v)", v, ok, tt.wantVal, tt.wantOK)
			}
		})
	}
}

func TestAttributeEvaluator_FieldMatchers(t *testing.T) {
	cfg := config.EffortConfig{
		Strategy: "attribute",
		Attribute: []config.EffortMatcher{
			{Query: "field:Size/XS", Value: 1},
			{Query: "field:Size/S", Value: 2},
			{Query: "field:Size/M", Value: 3},
			{Query: "field:Size/L", Value: 5},
			{Query: "field:Size/XL", Value: 8},
		},
	}

	e, err := NewEvaluator(cfg)
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}

	tests := []struct {
		name    string
		item    Item
		wantVal float64
		wantOK  bool
	}{
		{
			name:    "matches XS",
			item:    Item{Fields: map[string]string{"Size": "XS"}},
			wantVal: 1, wantOK: true,
		},
		{
			name:    "matches M case insensitive",
			item:    Item{Fields: map[string]string{"size": "m"}},
			wantVal: 3, wantOK: true,
		},
		{
			name:    "no match = not assessed",
			item:    Item{Fields: map[string]string{"Priority": "High"}},
			wantVal: 0, wantOK: false,
		},
		{
			name:    "nil fields = not assessed",
			item:    Item{},
			wantVal: 0, wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok := e.Evaluate(tt.item)
			if v != tt.wantVal || ok != tt.wantOK {
				t.Errorf("Evaluate() = (%.0f, %v), want (%.0f, %v)", v, ok, tt.wantVal, tt.wantOK)
			}
		})
	}
}

func TestHasFieldMatchers(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.EffortConfig
		want bool
	}{
		{"count strategy", config.EffortConfig{Strategy: "count"}, false},
		{"attribute no field", config.EffortConfig{Strategy: "attribute", Attribute: []config.EffortMatcher{{Query: "label:size/S", Value: 2}}}, false},
		{"attribute with field", config.EffortConfig{Strategy: "attribute", Attribute: []config.EffortMatcher{{Query: "field:Size/M", Value: 3}}}, true},
		{"mixed matchers", config.EffortConfig{Strategy: "attribute", Attribute: []config.EffortMatcher{{Query: "label:bug", Value: 1}, {Query: "field:Size/L", Value: 5}}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasFieldMatchers(tt.cfg); got != tt.want {
				t.Errorf("HasFieldMatchers() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractFieldMatcherNames(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.EffortConfig
		want []string
	}{
		{"count strategy", config.EffortConfig{Strategy: "count"}, nil},
		{"no field matchers", config.EffortConfig{Strategy: "attribute", Attribute: []config.EffortMatcher{{Query: "label:bug", Value: 1}}}, nil},
		{"one field", config.EffortConfig{Strategy: "attribute", Attribute: []config.EffortMatcher{{Query: "field:Size/M", Value: 3}, {Query: "field:Size/L", Value: 5}}}, []string{"Size"}},
		{"two fields", config.EffortConfig{Strategy: "attribute", Attribute: []config.EffortMatcher{{Query: "field:Size/M", Value: 3}, {Query: "field:Priority/High", Value: 8}}}, []string{"Size", "Priority"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractFieldMatcherNames(tt.cfg)
			if len(got) != len(tt.want) {
				t.Fatalf("ExtractFieldMatcherNames() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ExtractFieldMatcherNames()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestNewEvaluator_InvalidStrategy(t *testing.T) {
	_, err := NewEvaluator(config.EffortConfig{Strategy: "invalid"})
	if err == nil {
		t.Fatal("expected error for unknown strategy")
	}
}

func TestNewEvaluator_InvalidMatcher(t *testing.T) {
	_, err := NewEvaluator(config.EffortConfig{
		Strategy: "attribute",
		Attribute: []config.EffortMatcher{
			{Query: "bad:matcher:syntax", Value: 1},
		},
	})
	if err == nil {
		t.Fatal("expected error for invalid matcher")
	}
}
