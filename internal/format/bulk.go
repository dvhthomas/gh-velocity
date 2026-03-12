package format

// JSONWindow is the JSON representation of a date window.
type JSONWindow struct {
	Since string `json:"since"`
	Until string `json:"until"`
}
