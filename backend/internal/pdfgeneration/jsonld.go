package pdfgeneration

import "encoding/json"

// InjectTitle sets the "title" key in a JSON-LD object from name.
// Returns the input unchanged when name is nil or empty.
func InjectTitle(jsonldBytes []byte, name *string) ([]byte, error) {
	if name == nil || *name == "" {
		return jsonldBytes, nil
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(jsonldBytes, &doc); err != nil {
		return nil, err
	}
	titleJSON, err := json.Marshal(*name)
	if err != nil {
		return nil, err
	}
	doc["title"] = json.RawMessage(titleJSON)
	return json.Marshal(doc)
}
