package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// decodeStrictStructuredJSON rejects unknown fields and trailing values for
// model-proposed business arguments. It is not a provider response parser.
func decodeStrictStructuredJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("structured output contains multiple JSON values")
		}
		return fmt.Errorf("decode trailing structured output: %w", err)
	}
	return nil
}

// The sole shared provider wire contract is LiteLLM Chat Completions.
type chatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Strict      bool           `json:"strict"`
	Parameters  map[string]any `json:"parameters"`
}
type chatCompletionTool struct {
	Type     string                     `json:"type"`
	Function chatCompletionToolFunction `json:"function"`
}
type chatResponseFormat struct {
	Type       string `json:"type"`
	JSONSchema struct {
		Name   string         `json:"name"`
		Strict bool           `json:"strict"`
		Schema map[string]any `json:"schema"`
	} `json:"json_schema"`
}
type chatCompletionRequest struct {
	Model             string                  `json:"model"`
	Messages          []chatCompletionMessage `json:"messages"`
	Tools             []chatCompletionTool    `json:"tools,omitempty"`
	ToolChoice        string                  `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool                   `json:"parallel_tool_calls,omitempty"`
	ResponseFormat    *chatResponseFormat     `json:"response_format,omitempty"`
	MaxTokens         int                     `json:"max_tokens"`
	Stream            bool                    `json:"stream"`
}

type capabilityCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}
type capabilityResponse struct {
	Calls []capabilityCall
}

type chatUsage struct {
	PromptTokens     int
	CompletionTokens int
	CachedTokens     int
}

// validateStrictSchema refuses malformed strict tool/schema definitions before
// they are sent to the gateway. Every object field is required, and optional
// semantics must be represented using nullable values.
func validateStrictSchema(schema map[string]any) error {
	t, _ := schema["type"].(string)
	if t == "object" {
		props, ok := schema["properties"].(map[string]any)
		if !ok || schema["additionalProperties"] != false {
			return errors.New("strict object requires properties and additionalProperties=false")
		}
		// json.Unmarshal into map[string]any produces []any, while Go-built
		// schemas use []string. Both encode to the identical JSON array.
		var required []string
		switch raw := schema["required"].(type) {
		case []string:
			required = raw
		case []any:
			for _, entry := range raw {
				name, ok := entry.(string)
				if !ok {
					return errors.New("strict object has non-string required field")
				}
				required = append(required, name)
			}
		default:
			return errors.New("strict object requires explicit required array")
		}
		if len(required) != len(props) {
			return errors.New("strict object requires every declared property")
		}
		seen := make(map[string]bool, len(required))
		for _, name := range required {
			if seen[name] {
				return errors.New("duplicate required schema field")
			}
			if _, ok := props[name]; !ok {
				return fmt.Errorf("unknown required schema field %q", name)
			}
			seen[name] = true
		}
		for name, value := range props {
			child, ok := value.(map[string]any)
			if !ok || !seen[name] {
				return fmt.Errorf("invalid or non-required schema property %q", name)
			}
			if err := validateStrictSchema(child); err != nil {
				return fmt.Errorf("property %s: %w", name, err)
			}
		}
	}
	if t == "array" {
		items, ok := schema["items"].(map[string]any)
		if !ok {
			return errors.New("strict array requires items schema")
		}
		return validateStrictSchema(items)
	}
	return nil
}

func capabilityChatRequest(model, system, input string, tools []customerSalesTool, maxTokens int) (chatCompletionRequest, error) {
	if strings.TrimSpace(model) == "" || maxTokens <= 0 || len(tools) == 0 {
		return chatCompletionRequest{}, errors.New("capability request requires model, output budget and offered tools")
	}
	req := chatCompletionRequest{Model: model,
		Messages:   []chatCompletionMessage{{Role: "system", Content: system}, {Role: "user", Content: input}},
		ToolChoice: "required", MaxTokens: maxTokens, Stream: false,
	}
	noParallel := false
	req.ParallelToolCalls = &noParallel
	for _, tool := range tools {
		if tool.Type != "function" || strings.TrimSpace(tool.Name) == "" {
			return chatCompletionRequest{}, errors.New("invalid offered capability")
		}
		if err := validateStrictSchema(tool.Parameters); err != nil {
			return chatCompletionRequest{}, fmt.Errorf("capability %s schema: %w", tool.Name, err)
		}
		req.Tools = append(req.Tools, chatCompletionTool{Type: "function", Function: chatCompletionToolFunction{
			Name: tool.Name, Description: tool.Description, Strict: true, Parameters: tool.Parameters,
		}})
	}
	return req, nil
}

// validateStrictArguments enforces the exposed schema on the RETURNED model
// arguments too. An upstream model is not trusted to honor strict mode.
func validateStrictArguments(raw []byte, schema map[string]any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return errors.New("invalid structured model arguments")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("trailing structured model arguments")
	}
	return validateStrictValue(value, schema)
}

func validateStrictValue(value any, schema map[string]any) error {
	t, typeString := schema["type"].(string)
	allowed := []string(nil)
	if typeString {
		allowed = append(allowed, t)
	} else {
		switch list := schema["type"].(type) {
		case []string:
			allowed = append(allowed, list...)
		case []any:
			for _, item := range list {
				label, ok := item.(string)
				if !ok {
					return errors.New("invalid schema type")
				}
				allowed = append(allowed, label)
			}
		default:
			return errors.New("schema does not declare a type")
		}
	}
	okType := false
	for _, declared := range allowed {
		switch declared {
		case "null":
			okType = okType || value == nil
		case "object":
			_, ok := value.(map[string]any)
			okType = okType || ok
		case "array":
			_, ok := value.([]any)
			okType = okType || ok
		case "string":
			_, ok := value.(string)
			okType = okType || ok
		case "boolean":
			_, ok := value.(bool)
			okType = okType || ok
		case "integer":
			if n, ok := value.(json.Number); ok && !strings.ContainsAny(string(n), ".eE") {
				_, err := n.Int64()
				okType = okType || err == nil
			}
		case "number":
			_, ok := value.(json.Number)
			okType = okType || ok
		default:
			return errors.New("unsupported schema type")
		}
	}
	if !okType {
		return errors.New("model argument violates schema type")
	}
	if choices, found := schema["enum"]; found {
		encodedValue, _ := json.Marshal(value)
		match := false
		switch enums := choices.(type) {
		case []any:
			for _, candidate := range enums {
				test, _ := json.Marshal(candidate)
				if bytes.Equal(test, encodedValue) {
					match = true
					break
				}
			}
		case []string:
			for _, candidate := range enums {
				test, _ := json.Marshal(candidate)
				if bytes.Equal(test, encodedValue) {
					match = true
					break
				}
			}
		}
		if !match {
			return errors.New("model argument is not in allowed enum")
		}
	}
	if object, ok := value.(map[string]any); ok {
		props, _ := schema["properties"].(map[string]any)
		if len(object) != len(props) {
			return errors.New("model object is missing or adding fields")
		}
		for key, spec := range props {
			item, present := object[key]
			if !present {
				return fmt.Errorf("missing required model argument %q", key)
			}
			child, ok := spec.(map[string]any)
			if !ok {
				return errors.New("invalid nested schema")
			}
			if err := validateStrictValue(item, child); err != nil {
				return fmt.Errorf("model argument %s: %w", key, err)
			}
		}
	}
	if array, ok := value.([]any); ok {
		items, ok := schema["items"].(map[string]any)
		if !ok {
			return errors.New("invalid array schema")
		}
		for i, item := range array {
			if err := validateStrictValue(item, items); err != nil {
				return fmt.Errorf("model array item %d: %w", i, err)
			}
		}
	}
	return nil
}
