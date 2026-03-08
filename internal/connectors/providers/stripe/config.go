package stripe

type webhookPayload struct {
	Type    string `json:"type"`
	Account string `json:"account"`
}
