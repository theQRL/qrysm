package keyhandling

type KeystoreModule struct {
	Function string         `json:"function"`
	Params   map[string]any `json:"params"`
	Message  string         `json:"message"`
}
