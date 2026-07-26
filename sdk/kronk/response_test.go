package kronk

import (
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

	got := convertInputToMessages(input)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("convertInputToMessages mismatch (-want +got):\n%s", diff)
	}
}
