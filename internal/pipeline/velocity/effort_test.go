package velocity

import (
	"testing"

	"github.com/dvhthomas/gh-velocity/internal/config"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

func ptr(f float64) *float64 { return &f }

// TestVelocityEvaluatorAdapter verifies the adapter correctly converts
// model.VelocityItem to effort.Item and delegates to the shared evaluator.
func TestVelocityEvaluatorAdapter(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.EffortConfig
		item    model.VelocityItem
		wantVal float64
		wantOK  bool
	}{
		{
			name: "count always returns 1",
			cfg:  config.EffortConfig{Strategy: "count"},
			item: model.VelocityItem{Number: 1, Title: "anything"},
			wantVal: 1, wantOK: true,
		},
		{
			name: "attribute matches label",
			cfg: config.EffortConfig{
				Strategy:  "attribute",
				Attribute: []config.EffortMatcher{{Query: "label:size/S", Value: 2}},
			},
			item:    model.VelocityItem{Number: 2, Labels: []string{"size/S"}},
			wantVal: 2, wantOK: true,
		},
		{
			name: "attribute no match",
			cfg: config.EffortConfig{
				Strategy:  "attribute",
				Attribute: []config.EffortMatcher{{Query: "label:size/S", Value: 2}},
			},
			item:    model.VelocityItem{Number: 3, Labels: []string{"bug"}},
			wantVal: 0, wantOK: false,
		},
		{
			name:    "numeric positive",
			cfg:     config.EffortConfig{Strategy: "numeric"},
			item:    model.VelocityItem{Number: 4, Effort: ptr(5)},
			wantVal: 5, wantOK: true,
		},
		{
			name:    "numeric nil",
			cfg:     config.EffortConfig{Strategy: "numeric"},
			item:    model.VelocityItem{Number: 5, Effort: nil},
			wantVal: 0, wantOK: false,
		},
		{
			name:    "numeric negative",
			cfg:     config.EffortConfig{Strategy: "numeric"},
			item:    model.VelocityItem{Number: 6, Effort: ptr(-3)},
			wantVal: 0, wantOK: false,
		},
		{
			name: "attribute field matcher",
			cfg: config.EffortConfig{
				Strategy:  "attribute",
				Attribute: []config.EffortMatcher{{Query: "field:Size/M", Value: 3}},
			},
			item:    model.VelocityItem{Number: 7, Fields: map[string]string{"Size": "M"}},
			wantVal: 3, wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, err := NewEffortEvaluator(tt.cfg)
			if err != nil {
				t.Fatalf("NewEffortEvaluator: %v", err)
			}
			v, ok := e.Evaluate(tt.item)
			if v != tt.wantVal || ok != tt.wantOK {
				t.Errorf("Evaluate(#%d) = (%.1f, %v), want (%.1f, %v)", tt.item.Number, v, ok, tt.wantVal, tt.wantOK)
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
