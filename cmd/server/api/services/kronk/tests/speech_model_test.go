//go:build speech_integration

package chatapi_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
)

func Test_API_SpeechModels(t *testing.T) {
	test := apitest.NewWithModelConfig(t, "Test_API_SpeechModels", "testdata/model_config.yaml")
	token := speechToken(t, test, false, "speech")
	for _, id := range []string{
		"ggml-org/chatterbox-turbo-Q8_0", "ggml-org/Qwen3-TTS-12Hz-1.7B-Base-Q8_0",
		"ggml-org/Kokoro-82M-F32", "ggml-org/tiny-voca-F32",
	} {
		t.Run(id, func(t *testing.T) {
			var wav []byte
			var table []apitest.Table
			for _, format := range []string{"", "wav", "pcm"} {
				// Ignored controls must not alter the SDK's conditioning or limits.
				fields := ""
				if format != "" {
					fields = fmt.Sprintf(`,"response_format":%q,"stream_format":"audio","speed":"ignored","instructions":{"style":"ignored"},"language":"es","max_frames":1`, format)
				}
				body := fmt.Sprintf(`{"model":%q,"input":"Your turn!","voice":"default"%s}`, id, fields)
				name := format
				if name == "" {
					name = "default"
				}
				table = append(table, apitest.Table{
					Name: name, URL: "/v1/audio/speech", Method: http.MethodPost,
					Token: token, Headers: map[string]string{"Content-Type": "application/json"},
					RawBody: []byte(body), StatusCode: http.StatusOK,
					ResponseCmpFunc: func(header http.Header, audio []byte) string {
						if format == "pcm" {
							if header.Get("Content-Type") != "audio/pcm" || len(wav) <= 44 || !bytes.Equal(audio, wav[44:]) {
								return "PCM response does not match the WAV samples"
							}
							return ""
						}
						if header.Get("Content-Type") != "audio/wav" || len(audio) <= 44 || string(audio[:4]) != "RIFF" || string(audio[8:12]) != "WAVE" ||
							binary.LittleEndian.Uint16(audio[20:22]) != 1 || binary.LittleEndian.Uint16(audio[22:24]) != 1 ||
							binary.LittleEndian.Uint32(audio[24:28]) != 24000 || binary.LittleEndian.Uint16(audio[34:36]) != 16 ||
							binary.LittleEndian.Uint32(audio[40:44]) != uint32(len(audio)-44) {
							return "response is not complete 24 kHz PCM16 mono WAV"
						}
						if format == "" {
							wav = bytes.Clone(audio)
						} else if !bytes.Equal(audio, wav) {
							return "explicit WAV differs from the default response"
						}
						return ""
					},
				})
			}
			test.Run(t, table, "speech-audio")
			test.Run(t, []apitest.Table{
				{
					Name: "unknown-voice", URL: "/v1/audio/speech", Method: http.MethodPost, Token: token,
					RawBody:    []byte(fmt.Sprintf(`{"model":%q,"input":"Hello","voice":"not-a-voice"}`, id)),
					StatusCode: http.StatusBadRequest, GotResp: &errs.Error{}, ResponseCmpFunc: jsonContentType,
					CmpFunc: func(got, _ any) string {
						if g := got.(*errs.Error); !g.Code.Equal(errs.InvalidArgument) || !strings.Contains(g.Message, "voice") {
							return fmt.Sprintf("got %+v, want invalid voice error", g)
						}
						return ""
					},
				},
				{
					Name: "NUL-input", URL: "/v1/audio/speech", Method: http.MethodPost, Token: token,
					RawBody:    []byte(fmt.Sprintf(`{"model":%q,"input":"Hello\u0000","voice":"default"}`, id)),
					StatusCode: http.StatusBadRequest, GotResp: &errs.Error{}, ResponseCmpFunc: jsonContentType,
					CmpFunc: func(got, _ any) string {
						if g := got.(*errs.Error); !g.Code.Equal(errs.InvalidArgument) {
							return fmt.Sprintf("got %+v, want invalid input error", g)
						}
						return ""
					},
				},
			}, "speech-errors")

			// The table runner intentionally uses fresh request contexts; keep
			// this expired-context check separate from ordinary HTTP tables.
			t.Run("expired-request", func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), -time.Second)
				defer cancel()
				body := fmt.Sprintf(`{"model":%q,"input":"Hello","voice":"default"}`, id)
				r := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", strings.NewReader(body)).WithContext(ctx)
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("Authorization", "Bearer "+token)
				w := httptest.NewRecorder()
				test.ServeHTTP(w, r)
				if w.Code != http.StatusGatewayTimeout || w.Header().Get("Content-Type") != "application/json" {
					t.Errorf("expired request: got status=%d type=%s body=%s, want 504 JSON error", w.Code, w.Header().Get("Content-Type"), w.Body)
				}
			})
		})
	}
}
