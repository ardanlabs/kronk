package apitest

import "encoding/json"

// Table represent fields needed for running an api test.
//
// Input is JSON-marshalled into the request body. RawBody, when set,
// is used as the request body verbatim and Input is ignored — needed
// for endpoints like /v1/audio/transcriptions that accept
// multipart/form-data. Pair RawBody with a Content-Type header in
// Headers so the server sees the right MIME type.
type Table struct {
	Name          string
	SkipInGH      bool
	URL           string
	Token         string
	Headers       map[string]string
	Method        string
	StatusCode    int
	Input         any
	RawBody       []byte
	GotResp       any
	ExpResp       any
	CmpFunc       func(got any, exp any) string
	StreamCmpFunc func(events []json.RawMessage) string
	// StreamResponseOffset selects which SSE data event to decode for CmpFunc,
	// counting backward from the final non-[DONE] event. The default is 0.
	StreamResponseOffset int
	// RequireDone requires exactly one [DONE] event and no later data events.
	RequireDone bool
}
