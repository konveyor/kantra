package testing

import (
	"encoding/json"
	"os"
	"testing"
)

func TestGenerateTestsSchema(t *testing.T) {
	got, err := GenerateTestsSchema()
	if err != nil {
		t.Errorf("GenerateTestsSchema() error = %v", err)
		return
	}
	wantContent, err := os.ReadFile("../../test-schema.json")
	if err != nil {
		t.Errorf("failed reading expected schema file ../schema.json")
		return
	}
	gotContent, err := json.MarshalIndent(got, "", "\t")
	if err != nil {
		t.Errorf("failed unmarshaling expected schema file ../../test-schema.json")
		return
	}
	if string(gotContent) != string(wantContent) {
		t.Errorf("GenerateTestsSchema() want schema \n%v, got \n%v", string(gotContent), string(wantContent))
	}
}
