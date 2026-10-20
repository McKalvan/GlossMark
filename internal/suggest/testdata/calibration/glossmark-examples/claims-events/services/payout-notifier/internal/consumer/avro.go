package consumer

import (
	"encoding/json"

	"github.com/confluentinc/confluent-kafka-go/v2/schemaregistry"
	"github.com/confluentinc/confluent-kafka-go/v2/schemaregistry/serde"
	"github.com/confluentinc/confluent-kafka-go/v2/schemaregistry/serde/avrov2"
)

// AvroDecoder decodes registry-framed Avro values into generic maps.
type AvroDecoder struct {
	client schemaregistry.Client
	inner  *avrov2.Deserializer
}

// NewAvroDecoder connects to the schema registry.
func NewAvroDecoder(url string) (*AvroDecoder, error) {
	client, err := schemaregistry.NewClient(schemaregistry.NewConfig(url))
	if err != nil {
		return nil, err
	}
	d, err := avrov2.NewDeserializer(client, serde.ValueSerde, avrov2.NewDeserializerConfig())
	if err != nil {
		return nil, err
	}
	return &AvroDecoder{client: client, inner: d}, nil
}

// Decode implements Decoder.
func (a *AvroDecoder) Decode(topic string, value []byte) (string, map[string]any, error) {
	info, err := a.inner.GetSchema(topic, value)
	if err != nil {
		return "", nil, err
	}
	var fields map[string]any
	if err := a.inner.DeserializeInto(topic, value, &fields); err != nil {
		return "", nil, err
	}
	return recordName(info.Schema), fields, nil
}

func recordName(schema string) string {
	var head struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}
	if err := json.Unmarshal([]byte(schema), &head); err != nil || head.Name == "" {
		return ""
	}
	return head.Namespace + "." + head.Name
}
