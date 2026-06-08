package types

/*
MetadataMap converts Metadata to a map for backends that store JSON properties.
*/
func (m Metadata) Map() map[string]any {
	return map[string]any{
		"id":        m.ID,
		"source":    m.Source,
		"timestamp": m.Timestamp,
	}
}
