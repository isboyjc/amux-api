package constant_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
)

func TestAliVideoDefaultModelParamSchemas(t *testing.T) {
	for _, modelName := range []string{
		"happyhorse-1.1-t2v",
		"happyhorse-1.0-t2v",
		"happyhorse-1.1-i2v",
		"happyhorse-1.0-i2v",
		"happyhorse-1.1-r2v",
		"happyhorse-1.0-r2v",
		"happyhorse-1.0-video-edit",
		"wan2.7-i2v-2026-04-25",
	} {
		schema := constant.GetDefaultModelParamSchema(modelName)
		if schema == "" {
			t.Fatalf("missing default schema for %s", modelName)
		}
		var parsed map[string]interface{}
		if err := common.UnmarshalJsonStr(schema, &parsed); err != nil {
			t.Fatalf("invalid schema for %s: %v", modelName, err)
		}
		if parsed["type"] != "object" {
			t.Fatalf("schema type for %s=%v", modelName, parsed["type"])
		}
	}
}

func TestHappyHorseMediaSchemas(t *testing.T) {
	tests := []struct {
		model       string
		property    string
		contentRole string
		maxItems    float64
		required    bool
	}{
		{model: "happyhorse-1.1-i2v", property: "first_frame", contentRole: "first_frame", required: true},
		{model: "happyhorse-1.1-r2v", property: "reference_images", contentRole: "reference_image", maxItems: 9, required: true},
		{model: "happyhorse-1.0-video-edit", property: "source_video", contentRole: "reference_video", required: true},
		{model: "happyhorse-1.0-video-edit", property: "reference_images", contentRole: "reference_image", maxItems: 5},
	}
	for _, test := range tests {
		var schema map[string]interface{}
		if err := common.UnmarshalJsonStr(constant.GetDefaultModelParamSchema(test.model), &schema); err != nil {
			t.Fatalf("parse schema for %s: %v", test.model, err)
		}
		properties, _ := schema["properties"].(map[string]interface{})
		property, _ := properties[test.property].(map[string]interface{})
		if property["x-content-role"] != test.contentRole {
			t.Fatalf("content role for %s=%v", test.model, property["x-content-role"])
		}
		if test.maxItems > 0 && property["maxItems"] != test.maxItems {
			t.Fatalf("maxItems for %s=%v", test.model, property["maxItems"])
		}
		if test.required {
			requiredProperties, ok := schema["required"].([]interface{})
			if !ok {
				t.Fatalf("required list missing for %s", test.model)
			}
			found := false
			for _, requiredProperty := range requiredProperties {
				if requiredProperty == test.property {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("required property %s missing for %s", test.property, test.model)
			}
		}
	}
}

func TestHappyHorseSchemasDoNotExpose480P(t *testing.T) {
	for _, modelName := range []string{
		"happyhorse-1.1-t2v",
		"happyhorse-1.0-t2v",
		"happyhorse-1.1-i2v",
		"happyhorse-1.0-i2v",
		"happyhorse-1.1-r2v",
		"happyhorse-1.0-r2v",
		"happyhorse-1.0-video-edit",
	} {
		var schema struct {
			Properties map[string]struct {
				Enum []string `json:"enum"`
			} `json:"properties"`
		}
		if err := common.UnmarshalJsonStr(constant.GetDefaultModelParamSchema(modelName), &schema); err != nil {
			t.Fatalf("parse schema for %s: %v", modelName, err)
		}
		resolution := schema.Properties["resolution"].Enum
		if len(resolution) != 2 || resolution[0] != "720P" || resolution[1] != "1080P" {
			t.Fatalf("resolution enum for %s=%v, want [720P 1080P]", modelName, resolution)
		}
	}
}
