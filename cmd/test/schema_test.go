package test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestGenerateTestsSchema(t *testing.T) {
	got, err := GenerateTestsSchema()
	if err != nil {
		t.Errorf("GenerateTestsSchema() error = %v", err)
		return
	}
	const goldenPath = "examples/test-schema.json"
	wantContent, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Errorf("failed reading golden schema file %s: %v", goldenPath, err)
		return
	}
	gotContent, err := json.MarshalIndent(got, "", "\t")
	if err != nil {
		t.Errorf("failed marshaling generated schema: %v", err)
		return
	}
	if strings.TrimSpace(string(gotContent)) != strings.TrimSpace(string(wantContent)) {
		t.Errorf("GenerateTestsSchema() want schema \n%v, got \n%v", string(wantContent), string(gotContent))
	}
}
