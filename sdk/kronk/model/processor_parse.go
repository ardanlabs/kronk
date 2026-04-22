package model

import "strings"

// parseToolCall routes tool call content to the appropriate model-specific
// parser based on the format of the accumulated content.
func (p *processor) parseToolCall(content string) []ResponseToolCall {

	// {"name":"get_weather", "arguments":{"location":"NYC"}
	if strings.HasPrefix(content, "{\"name\"") {
		return p.parseJSONToolCall(content)
	}

	// <function=get_weather>\n<parameter=location>\nNYC\n</parameter>\n</function>
	// <function=invoke_cli_command>\n<parameter=call>\ngo version\n</parameter>\n</function>
	if strings.HasPrefix(content, "<function=") {
		return parseQwenToolCall(content)
	}

	// get_weather<arg_key>location</arg_key><arg_value>NYC</arg_value>
	// GLM-style format with <arg_key>/<arg_value> tags
	if strings.Contains(content, "<arg_key>") {
		return parseGLMToolCall(content)
	}

	// [TOOL_CALLS]get_weather[ARGS]{"location": "NYC"}
	// Mistral/Devstral format
	if strings.Contains(content, "[TOOL_CALLS]") {
		return p.parseMistralToolCall(content)
	}

	// call:get_weather{location:<|"|>NYC<|"|>}
	// Gemma4 format with call: prefix and <|"|> escaped quotes
	if strings.Contains(content, "call:") {
		return p.parseGemmaToolCall(content)
	}

	return nil
}
