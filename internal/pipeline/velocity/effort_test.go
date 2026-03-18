package velocity

import (
	"testing"

	"github.com/dvhthomas/gh-velocity/internal/config"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

func ptr(f float64) *float64 { return &f }

func TestCountEvaluator(t *testing.T) {
	e := CountEvaluator{}
	items := []model.VelocityItem{
		{Number: 1, Title: "anything"},
		{Number: 2, Labels: []string{"bug"}},
		{Number: 3},
	}
	for _, item := range items {
		v, ok := e.Evaluate(item)
		if !ok || v != 1 {
			t.Errorf("CountEvaluator.Evaluate(#%d) = (%.0f, %v), want (1, true)", item.Number, v, ok)
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

	e, err := NewEffortEvaluator(cfg)
	if err != nil {
		t.Fatalf("NewEffortEvaluator: %v", err)
	}

	tests := []struct {
		name    string
		item    model.VelocityItem
		wantVal float64
		wantOK  bool
	}{
		{
			name:    "matches first",
			item:    model.VelocityItem{Number: 1, Labels: []string{"size/S"}},
			wantVal: 2, wantOK: true,
		},
		{
			name:    "matches second",
			item:    model.VelocityItem{Number: 2, Labels: []string{"size/M", "bug"}},
			wantVal: 3, wantOK: true,
		},
		{
			name:    "first match wins with overlapping labels",
			item:    model.VelocityItem{Number: 3, Labels: []string{"size/S", "size/L"}},
			wantVal: 2, wantOK: true,
		},
		{
			name:    "no match = not assessed",
			item:    model.VelocityItem{Number: 4, Labels: []string{"bug"}},
			wantVal: 0, wantOK: false,
		},
		{
			name:    "value zero is valid",
			item:    model.VelocityItem{Number: 5, Labels: []string{"chore"}},
			wantVal: 0, wantOK: true,
		},
		{
			name:    "empty labels = not assessed",
			item:    model.VelocityItem{Number: 6},
			wantVal: 0, wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok := e.Evaluate(tt.item)
			if v != tt.wantVal || ok != tt.wantOK {
				t.Errorf("Evaluate(#%d) = (%.0f, %v), want (%.0f, %v)", tt.item.Number, v, ok, tt.wantVal, tt.wantOK)
			}
		})
	}
}

func TestNumericEvaluator(t *testing.T) {
	e := NumericEvaluator{}

	tests := []struct {
		name    string
		item    model.VelocityItem
		wantVal float64
		wantOK  bool
	}{
		{
			name:    "nil = not assessed",
			item:    model.VelocityItem{Number: 1, Effort: nil},
			wantVal: 0, wantOK: false,
		},
		{
			name:    "zero is valid",
			item:    model.VelocityItem{Number: 2, Effort: ptr(0)},
			wantVal: 0, wantOK: true,
		},
		{
			name:    "positive value",
			item:    model.VelocityItem{Number: 3, Effort: ptr(5)},
			wantVal: 5, wantOK: true,
		},
		{
			name:    "fractional value",
			item:    model.VelocityItem{Number: 4, Effort: ptr(2.5)},
			wantVal: 2.5, wantOK: true,
		},
		{
			name:    "negative = not assessed",
			item:    model.VelocityItem{Number: 5, Effort: ptr(-3)},
			wantVal: 0, wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok := e.Evaluate(tt.item)
			if v != tt.wantVal || ok != tt.wantOK {
				t.Errorf("Evaluate(#%d) = (%.1f, %v), want (%.1f, %v)", tt.item.Number, v, ok, tt.wantVal, tt.wantOK)
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

	e, err := NewEffortEvaluator(cfg)
	if err != nil {
		t.Fatalf("NewEffortEvaluator: %v", err)
	}

	tests := []struct {
		name    string
		item    model.VelocityItem
		wantVal float64
		wantOK  bool
	}{
		{
			name:    "matches XS",
			item:    model.VelocityItem{Number: 1, Fields: map[string]string{"Size": "XS"}},
			wantVal: 1, wantOK: true,
		},
		{
			name:    "matches M case insensitive",
			item:    model.VelocityItem{Number: 2, Fields: map[string]string{"size": "m"}},
			wantVal: 3, wantOK: true,
		},
		{
			name:    "no match = not assessed",
			item:    model.VelocityItem{Number: 3, Fields: map[string]string{"Priority": "High"}},
			wantVal: 0, wantOK: false,
		},
		{
			name:    "nil fields = not assessed",
			item:    model.VelocityItem{Number: 4},
			wantVal: 0, wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok := e.Evaluate(tt.item)
			if v != tt.wantVal || ok != tt.wantOK {
				t.Errorf("Evaluate(#%d) = (%.0f, %v), want (%.0f, %v)", tt.item.Number, v, ok, tt.wantVal, tt.wantOK)
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

func TestNewEffortEvaluator_InvalidStrategy(t *testing.T) {
	_, err := NewEffortEvaluator(config.EffortConfig{Strategy: "invalid"})
	if err == nil {
		t.Fatal("expected error for unknown strategy")
	}
}

func TestNewEffortEvaluator_InvalidMatcher(t *testing.T) {
	_, err := NewEffortEvaluator(config.EffortConfig{
		Strategy: "attribute",
		Attribute: []config.EffortMatcher{
			{Query: "bad:matcher:syntax", Value: 1},
		},
	})
	if err == nil {
		t.Fatal("expected error for invalid matcher")
	}
}
