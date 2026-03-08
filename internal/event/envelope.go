package event

import "encoding/json"

func UnmarshalEvent(data []byte, evt *Event) error {
	return json.Unmarshal(data, evt)
}

func MarshalEvent(evt Event) ([]byte, error) {
	return json.Marshal(evt)
}
