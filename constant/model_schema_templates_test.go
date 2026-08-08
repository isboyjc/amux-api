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
