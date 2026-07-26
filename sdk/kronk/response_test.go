package kronk

import (
	"errors"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/go-cmp/cmp"
)

func TestConvertInputToMessagesRoleShapedImage(t *testing.T) {
	const imageURL = "data:image/jpeg;base64,aW1hZ2U="

	input := model.D{
		"input": []model.D{
			{
				"role": "user",
				"content": []model.D{
					{"type": "input_image", "image_url": imageURL},
					{"type": "input_text", "text": "describe this image"},
				},
			},
		},
	}

	want := model.D{
		"messages": []model.D{
			{
				"role": "user",
				"content": []model.D{
					{
						"type": "image_url",
						"image_url": model.D{
							"url": imageURL,
						},
					},
					{"type": "text", "text": "describe this image"},
				},
			},
		},
	}

	got, err := convertInputToMessages(input)
	if err != nil {
		t.Fatalf("convertInputToMessages: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("convertInputToMessages mismatch (-want +got):\n%s", diff)
	}
}

func TestConvertInputToMessagesRejectsFileInput(t *testing.T) {
	tests := []struct {
		name  string
		input model.D
	}{
		{
			name: "role-shaped input",
			input: model.D{
				"input": []model.D{
					{
						"role": "user",
						"content": []model.D{
							{"type": "input_file", "filename": "document.pdf", "file_data": "data:application/pdf;base64,JVBERg=="},
						},
					},
				},
			},
		},
		{
			name: "flat input",
			input: model.D{
				"input": []model.D{
					{"type": "input_file", "filename": "document.txt", "file_data": "dGV4dA=="},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := convertInputToMessages(tt.input)
			if err == nil {
				t.Fatal("convertInputToMessages: expected error")
			}
			if !errors.Is(err, model.ErrFileInputsUnsupported) {
				t.Errorf("error: got %v, want ErrFileInputsUnsupported", err)
			}
			if got, want := err.Error(), "convert-input-to-messages: file inputs are not currently supported"; got != want {
				t.Errorf("error: got %q, want %q", got, want)
			}
		})
	}
}
