package model

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// Objects represent the different types of data that is being processed.
const (
	ObjectChatUnknown   = "chat.unknown"
	ObjectChatText      = "chat.completion.chunk"
	ObjectChatTextFinal = "chat.completion"
	ObjectChatMedia     = "chat.media"
)

// Roles represent the different roles that can be used in a chat.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
	RoleTool      = "tool"
)

// FinishReasons represent the different reasons a response can be finished.
const (
	FinishReasonStop   = "stop"
	FinishReasonLength = "length"
	FinishReasonTool   = "tool_calls"
	FinishReasonError  = "error"
)

// =============================================================================

// ModelType represents the model architecture for batch engine state management.
type ModelType uint8

const (
	// ModelTypeDense is a standard transformer model. State cleanup uses
	// partial range delete (MemorySeqRm).
	ModelTypeDense ModelType = iota

	// ModelTypeMoE is a Mixture of Experts model. Same state cleanup as Dense
	// (partial range delete), different performance profile due to scattered
	// memory access from expert routing.
	ModelTypeMoE

	// ModelTypeHybrid is a model with Attention + Recurrent layers
	// (DeltaNet/SSM). State cleanup requires snapshot/restore because partial
	// range delete corrupts recurrent state.
	ModelTypeHybrid
)

// String returns a human-readable name for the model type.
func (mt ModelType) String() string {
	switch mt {
	case ModelTypeDense:
		return "dense"
	case ModelTypeMoE:
		return "moe"
	case ModelTypeHybrid:
		return "hybrid"
	default:
		return "unknown"
	}
}

// =============================================================================

// ModelInfo represents the model's card information.
type ModelInfo struct {
	ID            string
	HasProjection bool
	Desc          string
	Size          uint64
	VRAMTotal     int64
	SlotMemory    int64
	NSWA          int32 // Effective SWA window in tokens; zero means the model does not use SWA.
	Type          ModelType
	IsEmbedModel  bool
	IsRerankModel bool
	Metadata      map[string]string
	Template      Template
}

func (mi ModelInfo) String() string {
	var flags []string
	if mi.HasProjection {
		flags = append(flags, "projection")
	}
	if mi.IsEmbedModel {
		flags = append(flags, "embed")
	}
	if mi.IsRerankModel {
		flags = append(flags, "rerank")
	}

	flagStr := "none"
	if len(flags) > 0 {
		flagStr = strings.Join(flags, ", ")
	}

	sizeGB := float64(mi.Size) / (1000 * 1000 * 1000)

	return fmt.Sprintf("\nID[%s]\nDesc[%s]\nSize[%.2fGB]\nNSWA[%d]\nTemplate[%s]\nType[%s]\nFlags[%s]\n", mi.ID, mi.Desc, sizeGB, mi.NSWA, mi.Template.FileName, mi.Type, flagStr)
}

func toModelInfo(cfg Config, model llama.Model) ModelInfo {
	desc := llama.ModelDesc(model)
	size := llama.ModelSize(model)
	count := llama.ModelMetaCount(model)
	metadata := make(map[string]string)

	for i := range count {
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					return
				}
			}()

			key, ok := llama.ModelMetaKeyByIndex(model, i)
			if !ok {
				return
			}

			value, ok := llama.ModelMetaValStrByIndex(model, i)
			if !ok {
				return
			}

			metadata[key] = value
		}()
	}

	modelID := modelIDFromFiles(cfg.ModelFiles)

	isEmbedModel, isRerankModel := detectEmbedRerank(modelID)

	modelType := detectModelType(model, metadata)

	return ModelInfo{
		ID:            modelID,
		HasProjection: cfg.ProjFile != "",
		Desc:          desc,
		Size:          size,
		NSWA:          llama.ModelNSWA(model),
		Type:          modelType,
		IsEmbedModel:  isEmbedModel,
		IsRerankModel: isRerankModel,
		Metadata:      metadata,
	}
}

var splitPattern = regexp.MustCompile(`-\d+-of-\d+$`)

// quantSuffixPattern matches a trailing GGUF quantization tag preceded by '.'
// or '-'. It covers the common formats shipped by community quantizers:
//
//   - Standard k-quants:   .Q8_0  -Q4_K_M  -Q4_K_S  .Q5_K_M  .Q6_K
//   - Block k-quants:      .Q4_0_4_4  .Q4_0_4_8  .Q4_0_8_8
//   - i-quants:            .IQ4_NL  .IQ3_XXS  -IQ2_M
//   - Unsloth dynamic:     -UD-Q8_K_XL  -UD-Q4_K_XL
//   - Float variants:      .f16  .f32  -bf16
//
// The body of a Q/IQ tag is followed by zero or more "_<digits-or-letters>"
// segments (Q8, Q8_0, Q4_K, Q4_K_M, Q4_0_4_4, IQ4_NL, ...). The pattern is
// anchored at end-of-string so it only strips the trailing suffix, leaving
// everything else intact (model family, parameter count, role tags).
var quantSuffixPattern = regexp.MustCompile(`(?i)[.\-](UD-)?(Q\d+(_[0-9A-Z]+)*|IQ\d+(_[0-9A-Z]+)*|bf16|f16|f32)$`)

// stripQuantSuffix returns modelID with a single trailing quantization suffix
// removed when present. It is used by the template auto-discovery resolver so
// "Qwopus3.5-4B-Coder.Q8_0" can be matched against a "Qwopus3.5-4B-Coder.jinja"
// template that applies to every quant of the same base model.
func stripQuantSuffix(modelID string) string {
	return quantSuffixPattern.ReplaceAllString(modelID, "")
}

func modelIDFromFiles(modelFiles []string) string {
	switch len(modelFiles) {
	case 1:
		return strings.TrimSuffix(filepath.Base(modelFiles[0]), path.Ext(modelFiles[0]))

	default:
		name := strings.TrimSuffix(filepath.Base(modelFiles[0]), filepath.Ext(modelFiles[0]))
		return splitPattern.ReplaceAllString(name, "")
	}
}

// DetectModelTypeFromFiles loads a model from the given GGUF files, determines
// the architecture type, and immediately frees the model. It returns the
// ModelType, the raw general.architecture string from the GGUF metadata, and
// any error encountered during loading.
func DetectModelTypeFromFiles(modelFiles []string) (ModelType, string, error) {
	params := llama.ModelDefaultParams()
	params.NGpuLayers = 0

	var mdl llama.Model
	var err error

	switch len(modelFiles) {
	case 1:
		mdl, err = llama.ModelLoadFromFile(modelFiles[0], params)
	default:
		mdl, err = llama.ModelLoadFromSplits(modelFiles, params)
	}

	if err != nil {
		return ModelTypeDense, "", fmt.Errorf("loading model: %w", err)
	}
	defer llama.ModelFree(mdl)

	count := llama.ModelMetaCount(mdl)
	metadata := make(map[string]string)

	for i := range count {
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					return
				}
			}()

			key, ok := llama.ModelMetaKeyByIndex(mdl, i)
			if !ok {
				return
			}

			value, ok := llama.ModelMetaValStrByIndex(mdl, i)
			if !ok {
				return
			}

			metadata[key] = value
		}()
	}

	mt := detectModelType(mdl, metadata)

	return mt, metadata["general.architecture"], nil
}

// detectModelType determines the model architecture from llama.cpp detection
// and GGUF metadata. Hybrid is detected via llama.ModelIsHybrid (recurrent
// layers). MoE is detected via GGUF expert_count metadata (value > 0).
// Everything else is Dense.
func detectModelType(model llama.Model, metadata map[string]string) ModelType {
	switch {
	case llama.ModelIsHybrid(model):
		return ModelTypeHybrid

	case hasExperts(metadata):
		return ModelTypeMoE

	default:
		return ModelTypeDense
	}
}

// hasExperts checks GGUF metadata for an expert_count key with a value > 0.
// GGUF keys are typically prefixed with the architecture name (e.g.,
// "qwen2moe.expert_count", "llama.expert_count").
func hasExperts(metadata map[string]string) bool {
	for key, val := range metadata {
		if !strings.HasSuffix(key, ".expert_count") {
			continue
		}

		n, err := strconv.Atoi(val)
		if err == nil && n > 0 {
			return true
		}
	}

	return false
}

func detectEmbedRerank(modelID string) (isEmbed bool, isRerank bool) {
	nameLower := strings.ToLower(modelID)

	if strings.Contains(nameLower, "embed") {
		isEmbed = true
	}

	if strings.Contains(nameLower, "rerank") {
		isRerank = true
	}

	return isEmbed, isRerank
}

// =============================================================================

// D represents a generic docment of fields and values.
type D map[string]any

// Clone creates a copy of the document. Mutable JSON maps and slices are cloned
// recursively so callers can reuse a request after submitting it. Byte slices
// are shared as immutable media payloads; scalar values are shared by value.
func (d D) Clone() D {
	clone := make(D, len(d))
	for k, v := range d {
		clone[k] = cloneDocumentValue(v)
	}
	return clone
}

func cloneDocumentValue(v any) any {
	switch val := v.(type) {
	case D:
		return val.Clone()
	case map[string]any:
		return D(val).Clone()
	case []D:
		s := make([]D, len(val))
		for i, item := range val {
			s[i] = item.Clone()
		}
		return s
	case []map[string]any:
		s := make([]map[string]any, len(val))
		for i, item := range val {
			s[i] = D(item).Clone()
		}
		return s
	case []any:
		// Normalized multipart content and arbitrary template kwargs can carry
		// mutable JSON containers inside []any. Clone those containers while
		// continuing to share []byte media payloads as immutable values.
		s := make([]any, len(val))
		for i, item := range val {
			s[i] = cloneDocumentValue(item)
		}
		return s
	case []byte:
		// Media payloads are shared as immutable values to avoid copying large
		// buffers at the request boundary.
		return val
	default:
		return cloneJSONContainer(v)
	}
}

func cloneJSONContainer(v any) any {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return v
	}

	switch rv.Kind() {
	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String || rv.IsNil() {
			return v
		}

		clone := reflect.MakeMapWithSize(rv.Type(), rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			clone.SetMapIndex(iter.Key(), clonedReflectValue(iter.Value(), rv.Type().Elem()))
		}
		return clone.Interface()

	case reflect.Slice:
		if rv.IsNil() {
			return v
		}

		clone := reflect.MakeSlice(rv.Type(), rv.Len(), rv.Len())
		for i := range rv.Len() {
			clone.Index(i).Set(clonedReflectValue(rv.Index(i), rv.Type().Elem()))
		}
		return clone.Interface()
	}

	return v
}

func clonedReflectValue(value reflect.Value, target reflect.Type) reflect.Value {
	clone := reflect.ValueOf(cloneDocumentValue(value.Interface()))
	if clone.IsValid() && clone.Type().AssignableTo(target) {
		return clone
	}
	return value
}

// ShallowClone creates a copy of the top-level map and the messages slice.
// Individual message maps are shared with the original. Use this when
// downstream code treats message maps as read-only or performs its own
// copy-on-write when mutation is needed.
func (d D) ShallowClone() D {
	clone := make(D, len(d))
	maps.Copy(clone, d)

	if msgs, ok := clone["messages"].([]D); ok {
		newMsgs := make([]D, len(msgs))
		copy(newMsgs, msgs)
		clone["messages"] = newMsgs
	}

	return clone
}

// logSafeKeys defines the allowlist of fields that are safe to log.
// Fields like "messages" and "input" are excluded for privacy.
var logSafeKeys = []string{
	"dimensions",
	"dry_allowed_length",
	"dry_base",
	"dry_multiplier",
	"frequency_penalty",
	"logprobs",
	"max_completion_tokens",
	"max_output_tokens",
	"max_tokens",
	"min_p",
	"model",
	"n",
	"parallel_tool_calls",
	"presence_penalty",
	"repeat_last_n",
	"repeat_penalty",
	"return_documents",
	"seed",
	"store",
	"stream",
	"temperature",
	"tool_choice",
	"top_k",
	"top_logprobs",
	"top_n",
	"top_p",
	"truncate",
	"truncate_direction",
	"truncation",
}

// String returns a string representation of the document containing only
// fields that are safe to log. This excludes sensitive fields like messages
// and input which may contain private user data.
func (d D) String() string {
	var b strings.Builder

	for i, key := range logSafeKeys {
		v, ok := d[key]
		if !ok {
			continue
		}

		if i > 0 {
			b.WriteString(" ")
		}

		fmt.Fprintf(&b, "%s[%v]:", key, v)
	}

	return b.String()
}

func (d D) Messages() string {
	var b strings.Builder
	b.WriteString("\n")

	messages, _ := d["messages"].([]D)
	for i, m := range messages {
		role, _ := m["role"].(string)
		fmt.Fprintf(&b, "Message[%d] Role: %s\n", i, role)

		switch role {
		case "assistant":
			fmt.Fprintf(&b, "Message[%d] Content: %s\n", i, formatLogContent(m["content"]))
			if reasoning, exists := m["reasoning_content"]; exists {
				fmt.Fprintf(&b, "Message[%d] ReasoningContent: %s\n", i, formatLogContent(reasoning))
			}
			toolCalls, _ := m["tool_calls"].([]D)
			fmt.Fprintf(&b, "Message[%d] ToolCalls len=%d\n", i, len(toolCalls))
			for j, tc := range toolCalls {
				fmt.Fprintf(&b, "  tc[%d]: %#v\n", j, tc)
			}

		case "tool":
			fmt.Fprintf(&b, "Message[%d] tool_call_id: %v\n", i, m["tool_call_id"])
			fmt.Fprintf(&b, "Message[%d] Content: %s\n", i, formatLogContent(m["content"]))

		default:
			fmt.Fprintf(&b, "Message[%d] Content: %s\n", i, formatLogContent(m["content"]))
		}
	}

	return b.String()
}

// formatLogContent renders a message["content"] value for diagnostic logging
// without ever dumping raw media bytes. Text is intentionally not truncated
// because this output is only used when insecure logging is enabled. Binary
// content ([]byte, or []byte nested inside a normalized []any) is summarized
// as "BYTES (N bytes)".
func formatLogContent(content any) string {
	switch v := content.(type) {
	case nil:
		return "<nil>"
	case string:
		return v
	case []byte:
		return fmt.Sprintf("BYTES (%d bytes)", len(v))
	case []D:
		text := contentPartsText(v)
		return fmt.Sprintf("(%d parts, full text): %s", len(v), text)
	case []any:
		// Normalized multipart content (string and []byte parts). Never
		// %v-format the slice directly, so a single image []byte cannot
		// dump the entire decoded byte sequence.
		var text strings.Builder
		var byteBytes int
		var byteParts int
		for _, p := range v {
			switch pv := p.(type) {
			case string:
				text.WriteString(pv)
			case []byte:
				byteParts++
				byteBytes += len(pv)
			}
		}
		return fmt.Sprintf("(%d parts, %d media bytes in %d media parts, full text): %s",
			len(v), byteBytes, byteParts, text.String())
	default:
		return fmt.Sprintf("%v", v)
	}
}

// contentPartsText extracts and concatenates text from OpenAI-style content
// part arrays ([{"type":"text","text":"..."},...]).
func contentPartsText(parts []D) string {
	var b strings.Builder
	for _, part := range parts {
		if t, _ := part["type"].(string); t == "text" {
			if text, _ := part["text"].(string); text != "" {
				b.WriteString(text)
			}
		}
	}
	return b.String()
}

// TextMessage create a new text message.
func TextMessage(role string, content string) D {
	return D{
		"role":    role,
		"content": content,
	}
}

// TextMessageArray creates a new text message using the OpenAI array format
// where content is [{"type": "text", "text": "..."}].
func TextMessageArray(role string, content string) D {
	return D{
		"role": role,
		"content": []D{
			{"type": "text", "text": content},
		},
	}
}

// ImageMessage create a new media message.
//
// The image part is emitted before the text part inside the user turn. Many
// multi-modal chat templates (e.g. Qwen2-Audio, Qwen-VL) were trained with
// the media token preceding the user's question, and putting text first can
// cause the model to ignore the media or produce nonsense output.
func ImageMessage(text string, media []byte, typ string) []D {
	encoded := base64.StdEncoding.EncodeToString(media)
	url := fmt.Sprintf("data:image/%s;base64,%s", typ, encoded)

	return []D{
		{
			"role": "user",
			"content": []D{
				{
					"type": "image_url",
					"image_url": D{
						"url": url,
					},
				},
				{
					"type": "text",
					"text": text,
				},
			},
		},
	}
}

// AudioMessage create a new media message.
//
// The audio part is emitted before the text part inside a single user turn,
// matching the shape llama-mtmd-cli builds for "-p <text> --audio <file>".
// Audio LLMs whose text side uses a standard Instruct chat template (e.g.
// Ultravox on Llama-3.x-Instruct) refuse to acknowledge the media when the
// audio and text live in separate user turns, because the template renders
// back-to-back user headers that the base model was never trained on. The
// single-turn shape is also the layout ImageMessage and VideoMessage use.
func AudioMessage(text string, media []byte, typ string) []D {
	encoded := base64.StdEncoding.EncodeToString(media)
	data := fmt.Sprintf("data:audio/%s;base64,%s", typ, encoded)

	return []D{
		{
			"role": "user",
			"content": []D{
				{
					"type": "input_audio",
					"input_audio": D{
						"data": data,
					},
				},
				{
					"type": "text",
					"text": text,
				},
			},
		},
	}
}

// VideoMessage create a new media message.
//
// The video part is emitted before the text part inside the user turn. Many
// multi-modal chat templates (e.g. Qwen2-Audio, Qwen-VL) were trained with
// the media token preceding the user's question, and putting text first can
// cause the model to ignore the media or produce nonsense output.
func VideoMessage(text string, media []byte, typ string) []D {
	encoded := base64.StdEncoding.EncodeToString(media)
	url := fmt.Sprintf("data:video/%s;base64,%s", typ, encoded)

	return []D{
		{
			"role": "user",
			"content": []D{
				{
					"type": "video_url",
					"video_url": D{
						"url": url,
					},
				},
				{
					"type": "text",
					"text": text,
				},
			},
		},
	}
}

// DocumentArray creates a new document array and can apply the
// set of documents.
func DocumentArray(doc ...D) []D {
	msgs := make([]D, len(doc))
	copy(msgs, doc)
	return msgs
}

// Messages combines individual messages (D) and message groups ([]D) into
// a single []D suitable for use as the "messages" field on a request. This
// lets you mix helpers like TextMessage (returns D) with helpers like
// ImageMessage or AudioMessage (return []D) in a single call.
//
// Unsupported item types panic, which signals a programmer error at the
// call site.
func Messages(items ...any) []D {
	msgs := make([]D, 0, len(items))

	for _, item := range items {
		switch v := item.(type) {
		case D:
			msgs = append(msgs, v)
		case []D:
			msgs = append(msgs, v...)
		default:
			panic(fmt.Sprintf("model.Messages: unsupported type %T", item))
		}
	}

	return msgs
}

// MapToModelD converts a map[string]any to a D.
func MapToModelD(m map[string]any) D {
	d := make(D, len(m))

	for k, v := range m {
		d[k] = convertValue(v)
	}

	return d
}

func convertValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return MapToModelD(val)

	case []any:
		allMaps := true
		for _, elem := range val {
			if _, ok := elem.(map[string]any); !ok {
				allMaps = false
				break
			}
		}

		if allMaps {
			result := make([]D, len(val))
			for i, elem := range val {
				result[i] = convertValue(elem).(D)
			}
			return result
		}

		for i, elem := range val {
			val[i] = convertValue(elem)
		}

		return val

	default:
		return v
	}
}

// =============================================================================

// ToolCallArguments represents tool call arguments that marshal to a JSON
// string per OpenAI API spec, but can unmarshal from either a string or object.
type ToolCallArguments map[string]any

func (a ToolCallArguments) MarshalJSON() ([]byte, error) {
	if a == nil {
		a = ToolCallArguments{}
	}

	inner, err := json.Marshal(map[string]any(a))
	if err != nil {
		return nil, err
	}

	return json.Marshal(string(inner))
}

func (a *ToolCallArguments) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*a = nil
		return nil
	}

	// Try string first (OpenAI compliant format).
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}

		if s == "" {
			*a = nil
			return nil
		}

		var m map[string]any
		if err := decodeJSONWithNumber(s, &m); err != nil {
			return err
		}

		*a = m
		return nil
	}

	// Fall back to object (non-compliant but some clients send this).
	var m map[string]any
	if err := decodeJSONWithNumber(string(data), &m); err != nil {
		return err
	}

	*a = m
	return nil
}

func decodeJSONWithNumber(data string, value any) error {
	dec := json.NewDecoder(strings.NewReader(data))
	dec.UseNumber()

	if err := dec.Decode(value); err != nil {
		return err
	}

	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("unexpected data after JSON value")
		}
		return err
	}

	return nil
}

type ResponseToolCallFunction struct {
	Name      string            `json:"name"`
	Arguments ToolCallArguments `json:"arguments"`
}

type ResponseToolCall struct {
	ID       string                   `json:"id"`
	Index    int                      `json:"-"`
	Type     string                   `json:"type"`
	Function ResponseToolCallFunction `json:"function"`
	Status   int                      `json:"status,omitempty"`
	Raw      string                   `json:"raw,omitempty"`
	Error    string                   `json:"error,omitempty"`
}

// ResponseToolCallDeltaFunction represents an incremental function-call delta.
type ResponseToolCallDeltaFunction struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments"`
}

// ResponseToolCallDelta represents an incremental OpenAI tool-call delta.
type ResponseToolCallDelta struct {
	ID       string                        `json:"id,omitempty"`
	Index    int                           `json:"index"`
	Type     string                        `json:"type,omitempty"`
	Function ResponseToolCallDeltaFunction `json:"function"`
}

// ResponseMessage represents a single message in a response.
type ResponseMessage struct {
	Role           string                  `json:"role,omitempty"`
	Content        string                  `json:"content"`
	Reasoning      string                  `json:"reasoning_content,omitempty"`
	ToolCalls      []ResponseToolCall      `json:"tool_calls,omitempty"`
	ToolCallDeltas []ResponseToolCallDelta `json:"-"`
}

// MarshalJSON emits incremental and completed tool calls through the same
// OpenAI-compatible tool_calls wire field.
func (m ResponseMessage) MarshalJSON() ([]byte, error) {
	type responseMessage struct {
		Role      string `json:"role,omitempty"`
		Content   string `json:"content"`
		Reasoning string `json:"reasoning_content,omitempty"`
		ToolCalls any    `json:"tool_calls,omitempty"`
	}

	var toolCalls any
	switch {
	case len(m.ToolCallDeltas) > 0:
		toolCalls = m.ToolCallDeltas
	case len(m.ToolCalls) > 0:
		toolCalls = m.ToolCalls
	}

	return json.Marshal(responseMessage{
		Role:      m.Role,
		Content:   m.Content,
		Reasoning: m.Reasoning,
		ToolCalls: toolCalls,
	})
}

// Choice represents a single choice in a response.
type Choice struct {
	Index           int              `json:"index"`
	Message         *ResponseMessage `json:"message,omitempty"`
	Delta           *ResponseMessage `json:"delta,omitempty"`
	Logprobs        *Logprobs        `json:"logprobs,omitempty"`
	FinishReasonPtr *string          `json:"finish_reason"`
}

// FinishReason return the finish reason as an empty
// string if it is nil.
func (c Choice) FinishReason() string {
	if c.FinishReasonPtr == nil {
		return ""
	}
	return *c.FinishReasonPtr
}

// Usage provides token usage information for a chat completion request.
//
// CompletionTokens includes all generated tokens, including reasoning tokens.
// CompletionTokensDetails provides the reasoning-token subset.
// TotalTokens is the sum of PromptTokens and CompletionTokens.
//
// DraftAcceptanceRate is the ratio of accepted drafts to total drafts
// across the spec rounds that actually ran. It is "quality per round"
// and says nothing about how much of the request used speculation.
// DraftCoverage is the complementary "how much" metric: the fraction
// of emitted output positions produced through speculation. Together
// they distinguish "MTP ran the whole request at 94%" from "MTP ran
// for 4 rounds at 94% then was disabled and the rest was target-only"
// — the second case shows high DraftAcceptanceRate but low
// DraftCoverage. DraftDisableReason explains the latter case
// ("imc-hit", "mirror-error", or empty if MTP was never disabled).
type Usage struct {
	PromptTokens            int                     `json:"prompt_tokens"`
	PromptTokensDetails     PromptTokensDetails     `json:"prompt_tokens_details"`
	CompletionTokens        int                     `json:"completion_tokens"`
	CompletionTokensDetails CompletionTokensDetails `json:"completion_tokens_details"`
	TotalTokens             int                     `json:"total_tokens"`
	TokensPerSecond         float64                 `json:"tokens_per_second"`
	TimeToFirstTokenMS      float64                 `json:"time_to_first_token_ms"`
	DraftTokens             int                     `json:"draft_tokens,omitempty"`
	DraftAcceptedTokens     int                     `json:"draft_accepted_tokens,omitempty"`
	DraftAcceptanceRate     float64                 `json:"draft_acceptance_rate,omitempty"`
	DraftCoverage           float64                 `json:"draft_coverage,omitempty"`
	DraftDisableReason      string                  `json:"draft_disable_reason,omitempty"`
}

// PromptTokensDetails provides a breakdown of prompt tokens.
type PromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

// CompletionTokensDetails provides a breakdown of completion tokens.
type CompletionTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

// TopLogprob represents a single token with its log probability.
type TopLogprob struct {
	Token   string  `json:"token"`
	Logprob float32 `json:"logprob"`
	Bytes   []byte  `json:"bytes,omitempty"`
}

// ContentLogprob represents log probability information for a single token.
type ContentLogprob struct {
	Token       string       `json:"token"`
	Logprob     float32      `json:"logprob"`
	Bytes       []byte       `json:"bytes,omitempty"`
	TopLogprobs []TopLogprob `json:"top_logprobs,omitempty"`
}

// Logprobs contains log probability information for the response.
type Logprobs struct {
	Content []ContentLogprob `json:"content,omitempty"`
}

// ChatResponse represents output for inference models.
type ChatResponse struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"`
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	SystemFingerprint string   `json:"system_fingerprint"`
	Choices           []Choice `json:"choices"`
	Usage             *Usage   `json:"usage,omitempty"`
	internal          internalChatResponse
}

type internalChatResponse struct {
	cause error
}

func chatResponseDelta(id string, object string, model string, index int, content string, reasoning bool, logprob *ContentLogprob) ChatResponse {
	var logprobs *Logprobs
	if logprob != nil {
		logprobs = &Logprobs{Content: []ContentLogprob{*logprob}}
	}

	return ChatResponse{
		ID:                id,
		Object:            object,
		Created:           time.Now().Unix(),
		Model:             model,
		SystemFingerprint: "fp_kronk",
		Choices: []Choice{
			{
				Index: index,
				Delta: &ResponseMessage{
					Role:      RoleAssistant,
					Content:   forContent(content, reasoning),
					Reasoning: forReasoning(content, reasoning),
				},
				Logprobs:        logprobs,
				FinishReasonPtr: nil,
			},
		},
	}
}

func chatResponseToolCallDelta(id string, object string, model string, index int, delta ResponseToolCallDelta) ChatResponse {
	return ChatResponse{
		ID:                id,
		Object:            object,
		Created:           time.Now().Unix(),
		Model:             model,
		SystemFingerprint: "fp_kronk",
		Choices: []Choice{
			{
				Index: index,
				Delta: &ResponseMessage{
					Role:           RoleAssistant,
					ToolCallDeltas: []ResponseToolCallDelta{delta},
				},
			},
		},
	}
}

func forContent(content string, reasoning bool) string {
	if !reasoning {
		return content
	}

	return ""
}

func forReasoning(content string, reasoning bool) string {
	if reasoning {
		return content
	}

	return ""
}

func chatResponseFinal(id string, object string, model string, index int, content string, reasoning string, respToolCalls []ResponseToolCall, logprobsData []ContentLogprob, finishReason string, includeUsage bool, u Usage) ChatResponse {
	var logprobs *Logprobs
	if len(logprobsData) > 0 {
		logprobs = &Logprobs{Content: logprobsData}
	}
	var usage *Usage
	if includeUsage {
		usage = &u
	}

	msg := &ResponseMessage{
		Role:      RoleAssistant,
		Content:   content,
		Reasoning: reasoning,
		ToolCalls: respToolCalls,
	}

	// Streaming clients receive completed tool-call arguments in a preceding
	// nonterminal chunk. The terminal delta stays empty per the OpenAI protocol.
	delta := &ResponseMessage{}
	if finishReason == "" {
		finishReason = FinishReasonStop
		if len(respToolCalls) > 0 {
			finishReason = FinishReasonTool
		}
	}

	return ChatResponse{
		ID:                id,
		Object:            object,
		Created:           time.Now().Unix(),
		Model:             model,
		SystemFingerprint: "fp_kronk",
		Choices: []Choice{
			{
				Index:           index,
				Message:         msg,
				Delta:           delta,
				Logprobs:        logprobs,
				FinishReasonPtr: &finishReason,
			},
		},
		Usage: usage,
	}
}

func chatResponseUsage(resp ChatResponse, usage Usage) ChatResponse {
	resp.Choices = []Choice{}
	resp.Usage = &usage
	return resp
}

func ChatResponseErr(id string, object string, model string, index int, err error, u Usage) ChatResponse {
	finishReason := FinishReasonError
	return ChatResponse{
		ID:                id,
		Object:            object,
		Created:           time.Now().Unix(),
		Model:             model,
		SystemFingerprint: "fp_kronk",
		Choices: []Choice{
			{
				Index: index,
				Delta: &ResponseMessage{
					Role:    RoleAssistant,
					Content: err.Error(),
				},
				FinishReasonPtr: &finishReason,
			},
		},
		Usage: &u,
		internal: internalChatResponse{
			cause: err,
		},
	}
}

// =============================================================================

// EmbedData represents the data associated with an embedding call.
type EmbedData struct {
	Object    string    `json:"object"`
	Index     int       `json:"index"`
	Embedding []float32 `json:"embedding"`
}

// EmbedUsage provides token usage information for embeddings.
type EmbedUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// EmbedReponse represents the output for an embedding call.
type EmbedReponse struct {
	Object  string      `json:"object"`
	Created int64       `json:"created"`
	Model   string      `json:"model"`
	Data    []EmbedData `json:"data"`
	Usage   EmbedUsage  `json:"usage"`
}

// =============================================================================

// RerankResult represents a single document's reranking result.
type RerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float32 `json:"relevance_score"`
	Document       string  `json:"document,omitempty"`
}

// RerankUsage provides token usage information for reranking.
type RerankUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// RerankResponse represents the output for a reranking call.
type RerankResponse struct {
	Object  string         `json:"object"`
	Created int64          `json:"created"`
	Model   string         `json:"model"`
	Data    []RerankResult `json:"data"`
	Usage   RerankUsage    `json:"usage"`
}

// =============================================================================

// TokenizeResponse represents the output for a tokenize call.
type TokenizeResponse struct {
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Tokens  int    `json:"tokens"`
}

// =============================================================================

type chatMessageURLData struct {
	// Only base64 encoded image is currently supported.
	URL string
}

type chatMessageRawData struct {
	// Only base64 encoded audio is currently supported.
	Data string
}

type chatMessageContent struct {
	Type      string
	Text      string
	ImageURL  chatMessageURLData
	VideoURL  chatMessageURLData
	AudioData chatMessageRawData
}

// chatMessage is the internal typed view of one user/assistant/tool message
// extracted from the raw request D. Content is one of:
//
//   - nil                          — no content (e.g. assistant tool_calls)
//   - string                       — plain text (or base64 string for Form1
//     when the SDK caller supplied bytes via plain string content)
//   - []byte                       — raw media bytes (Form1)
//   - []chatMessageContent         — typed multipart parts (Form2, OpenAI
//     image_url / video_url / input_audio)
type chatMessage struct {
	Role    string
	Content any
}

type chatMessages struct {
	Messages []chatMessage
}

// toChatMessages builds a typed chatMessages view of d["messages"] without
// going through a JSON marshal/unmarshal round-trip. It walks the D directly
// so:
//
//   - []byte content is preserved as []byte (no base64 round-trip just so
//     the caller can re-decode the same bytes a moment later in
//     detectMediaType);
//   - typed multipart parts ([]D / []any of maps with "type", "text",
//     "image_url", "video_url", "input_audio") are projected into
//     []chatMessageContent;
//   - text content is preserved as string.
//
// This is the single place where request shape is normalized for media
// detection. Other functions (detectMediaContent, toMediaMessage) consume
// chatMessages.Messages without re-parsing the underlying D.
func toChatMessages(d D) (chatMessages, error) {
	raw, ok := d["messages"]
	if !ok {
		return chatMessages{}, nil
	}

	msgs, err := coerceMessageSlice(raw)
	if err != nil {
		return chatMessages{}, err
	}

	out := chatMessages{Messages: make([]chatMessage, 0, len(msgs))}

	for i, msg := range msgs {
		var cm chatMessage
		if r, ok := msg["role"].(string); ok {
			cm.Role = r
		}

		c, hasContent := msg["content"]
		if !hasContent || c == nil {
			out.Messages = append(out.Messages, cm)
			continue
		}

		content, err := convertChatContent(c, i)
		if err != nil {
			return chatMessages{}, err
		}
		cm.Content = content

		out.Messages = append(out.Messages, cm)
	}

	return out, nil
}

// coerceMessageSlice accepts the common shapes that d["messages"] can hold
// after JSON decoding into a model.D: []D (the post-MapToModelD canonical
// shape), []any of map-likes (defensive — e.g. before validateDocument has
// tightened the shape), or []map[string]any.
func coerceMessageSlice(raw any) ([]D, error) {
	switch v := raw.(type) {
	case []D:
		return v, nil
	case []map[string]any:
		out := make([]D, len(v))
		for i, m := range v {
			out[i] = D(m)
		}
		return out, nil
	case []any:
		out := make([]D, 0, len(v))
		for i, item := range v {
			switch m := item.(type) {
			case D:
				out = append(out, m)
			case map[string]any:
				out = append(out, D(m))
			default:
				return nil, fmt.Errorf("messages[%d]: expected message map, got %T", i, item)
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("messages: expected []D, got %T", raw)
}

func convertChatContent(c any, msgIdx int) (any, error) {
	switch v := c.(type) {
	case string:
		return v, nil
	case []byte:
		return v, nil
	case []chatMessageContent:
		// Already in the target shape (e.g. test fixtures).
		return v, nil
	case []D:
		return chatMessagePartsFromMaps(v, msgIdx, dToMap)
	case []map[string]any:
		return chatMessagePartsFromMaps(v, msgIdx, identityMap)
	case []any:
		out := make([]chatMessageContent, 0, len(v))
		for j, p := range v {
			m, ok := mapFromPart(p)
			if !ok {
				return nil, fmt.Errorf("messages[%d].content[%d]: expected part map, got %T", msgIdx, j, p)
			}
			out = append(out, chatMessagePartFromMap(m))
		}
		return out, nil
	}
	return nil, fmt.Errorf("messages[%d].content: unsupported type %T", msgIdx, c)
}

func chatMessagePartsFromMaps[T any](parts []T, msgIdx int, toMap func(T) (map[string]any, bool)) ([]chatMessageContent, error) {
	out := make([]chatMessageContent, 0, len(parts))
	for j, p := range parts {
		m, ok := toMap(p)
		if !ok {
			return nil, fmt.Errorf("messages[%d].content[%d]: expected part map, got %T", msgIdx, j, p)
		}
		out = append(out, chatMessagePartFromMap(m))
	}
	return out, nil
}

func dToMap(d D) (map[string]any, bool)                   { return d, true }
func identityMap(m map[string]any) (map[string]any, bool) { return m, true }

func mapFromPart(p any) (map[string]any, bool) {
	switch v := p.(type) {
	case map[string]any:
		return v, true
	case D:
		return v, true
	}
	return nil, false
}

func chatMessagePartFromMap(m map[string]any) chatMessageContent {
	cm := chatMessageContent{}
	if t, ok := m["type"].(string); ok {
		cm.Type = t
	}
	if t, ok := m["text"].(string); ok {
		cm.Text = t
	}
	if u := urlDataFromAny(m["image_url"]); u != nil {
		cm.ImageURL = *u
	}
	if u := urlDataFromAny(m["video_url"]); u != nil {
		cm.VideoURL = *u
	}
	if a := rawDataFromAny(m["input_audio"]); a != nil {
		cm.AudioData = *a
	}
	return cm
}

func urlDataFromAny(v any) *chatMessageURLData {
	m, ok := mapFromPart(v)
	if !ok {
		return nil
	}
	if url, ok := m["url"].(string); ok {
		return &chatMessageURLData{URL: url}
	}
	return nil
}

func rawDataFromAny(v any) *chatMessageRawData {
	m, ok := mapFromPart(v)
	if !ok {
		return nil
	}
	if data, ok := m["data"].(string); ok {
		return &chatMessageRawData{Data: data}
	}
	return nil
}

// =============================================================================

// Template provides the template file name.
type Template struct {
	FileName string
	Script   string
}
