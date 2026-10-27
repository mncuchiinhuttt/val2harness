package configfile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
)

// JSONObject accepts only one object document, preserving unknown fields.
func JSONObject(data []byte) (map[string]any, error) {
	out := make(map[string]any)
	if data == nil {
		return out, nil
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := d.Decode(&out); err != nil {
		return nil, err
	}
	if out == nil {
		return nil, fmt.Errorf("settings must be an object")
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("settings must contain one JSON object")
	}
	return out, nil
}

// YAMLDocument preserves comments and rejects null/scalar/sequence/multiple roots.
func YAMLDocument(data []byte) (*yaml.Node, error) {
	if data == nil || len(bytes.TrimSpace(data)) == 0 {
		return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}, nil
	}
	var root yaml.Node
	d := yaml.NewDecoder(bytes.NewReader(data))
	if err := d.Decode(&root); err != nil {
		return nil, err
	}
	if len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("settings must be a YAML mapping")
	}
	var extra yaml.Node
	if err := d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("settings must contain one YAML document")
	}
	// Decode once to reject duplicate keys before any write while retaining nodes.
	var check map[string]any
	if err := root.Decode(&check); err != nil {
		return nil, err
	}
	// Materialize aliases before callers replace provider/model nodes. An anchor
	// may live in a removed node while an unrelated setting still refers to it.
	if err := expandAliases(&root); err != nil {
		return nil, err
	}
	return &root, nil
}

func expandAliases(node *yaml.Node) error {
	if node.Kind == yaml.AliasNode {
		var value any
		if err := node.Decode(&value); err != nil {
			return err
		}
		var expanded yaml.Node
		if err := expanded.Encode(value); err != nil {
			return err
		}
		expanded.HeadComment, expanded.LineComment, expanded.FootComment = node.HeadComment, node.LineComment, node.FootComment
		*node = expanded
	}
	for _, child := range node.Content {
		if err := expandAliases(child); err != nil {
			return err
		}
	}
	return nil
}

// MarshalYAML checks the final document as well as the original before publish.
func MarshalYAML(root *yaml.Node) ([]byte, error) {
	data, err := yaml.Marshal(root)
	if err != nil {
		return nil, err
	}
	if _, err := YAMLDocument(data); err != nil {
		return nil, fmt.Errorf("updated YAML is invalid: %w", err)
	}
	return data, nil
}

func ObjectField(root map[string]any, key string) (map[string]any, error) {
	value, exists := root[key]
	if !exists {
		child := make(map[string]any)
		root[key] = child
		return child, nil
	}
	child, ok := value.(map[string]any)
	if !ok || child == nil {
		return nil, fmt.Errorf("%s must be an object", key)
	}
	return child, nil
}

func YAMLField(root *yaml.Node, key string, kind yaml.Kind) (*yaml.Node, error) {
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			value := root.Content[i+1]
			if value.Kind != kind {
				return nil, fmt.Errorf("%s has an invalid container type", key)
			}
			return value, nil
		}
	}
	child := &yaml.Node{Kind: kind}
	SetYAMLField(root, key, child)
	return child, nil
}
func SetYAMLField(root *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			root.Content[i+1] = value
			return
		}
	}
	root.Content = append(root.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, value)
}
