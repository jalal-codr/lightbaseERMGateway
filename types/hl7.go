package types

type HL7Result struct {
	ObservationID  string `bson:"observation_id" json:"observation_id"`
	TestCode       string `bson:"test_code" json:"test_code"`
	TestName       string `bson:"test_name" json:"test_name"`
	Value          string `bson:"value" json:"value"`
	Units          string `bson:"units,omitempty" json:"units,omitempty"`
	ReferenceRange string `bson:"reference_range,omitempty" json:"reference_range,omitempty"`
	AbnormalFlags  string `bson:"abnormal_flags,omitempty" json:"abnormal_flags,omitempty"`
	Status         string `bson:"status" json:"status"`
	Timestamp      string `bson:"timestamp" json:"timestamp"`
}

type HL7Patient struct {
	ID   string `bson:"id" json:"id"`
	Name string `bson:"name" json:"name"`
}

type HL7Order struct {
	AccessionNumber string `bson:"accession_number" json:"accession_number"`
}

type HL7Payload struct {
	Source     string      `bson:"source" json:"source"`
	MessageID  string      `bson:"message_id" json:"message_id"`
	Patient    HL7Patient  `bson:"patient" json:"patient"`
	Order      HL7Order    `bson:"order" json:"order"`
	Results    []HL7Result `bson:"results" json:"results"`
	ReceivedAt string      `bson:"received_at" json:"received_at"`
}

type HL7Message struct {
	ID         string      `bson:"_id,omitempty" json:"id,omitempty"`
	Source     string      `bson:"source" json:"source"`
	MessageID  string      `bson:"message_id" json:"message_id"`
	Patient    HL7Patient  `bson:"patient" json:"patient"`
	Order      HL7Order    `bson:"order" json:"order"`
	Results    []HL7Result `bson:"results" json:"results"`
	ReceivedAt string      `bson:"received_at" json:"received_at"`
	CreatedAt  string      `bson:"created_at,omitempty" json:"created_at,omitempty"`
}
