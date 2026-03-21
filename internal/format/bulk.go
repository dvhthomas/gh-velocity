package format

// JSONWindow is the JSON representation of a date window.
type JSONWindow struct {
	Since string `json:"since"`
	Until string `json:"until"`
}

// JSONSort describes how a list of items is sorted.
type JSONSort struct {
	Field     string `json:"field"`
	Direction string `json:"direction"` // "desc" or "asc"
}
