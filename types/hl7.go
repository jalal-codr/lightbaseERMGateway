package types

type HL7Result struct {
	ObservationID  string `json:"observation_id"`
	TestCode       string `json:"test_code"`
	TestName       string `json:"test_name"`
	Value          string `json:"value"`
	Units          string `json:"units"`
	ReferenceRange string `json:"reference_range"`
	AbnormalFlags  string `json:"abnormal_flags"`
	Status         string `json:"status"`
	Timestamp      string `json:"timestamp"`
}

type HL7Payload struct {
	Source    string `json:"source"`
	MessageID string `json:"message_id"`
	Patient   struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"patient"`
	Order struct {
		AccessionNumber string `json:"accession_number"`
	} `json:"order"`
	Results    []HL7Result `json:"results"`
	ReceivedAt string      `json:"received_at"`
}
