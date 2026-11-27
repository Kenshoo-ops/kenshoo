package config

import (
	"errors"
	"io"

	"gopkg.in/yaml.v3"
)

var errSingleDocument = errors.New("config must contain a single YAML document")

// noMoreDocuments reports an error if the decoder has another YAML document with content.
// A trailing "---" with nothing after it is an empty document and is harmless. The decoder's
// own error is not wrapped: a YAML syntax error can quote the file.
func noMoreDocuments(dec *yaml.Decoder) error {
	for {
		var n yaml.Node
		err := dec.Decode(&n)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return errSingleDocument
		}
		if !isEmptyDocument(&n) {
			return errSingleDocument
		}
	}
}

func isEmptyDocument(n *yaml.Node) bool {
	if n.Kind == yaml.DocumentNode {
		if len(n.Content) == 0 {
			return true
		}
		n = n.Content[0]
	}
	return n.Kind == yaml.ScalarNode && n.ShortTag() == "!!null" && n.Value == ""
}
