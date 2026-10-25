package jsonfix

import (
	"encoding/json"
	"testing"
)

func TestRepairEscapesControlCharactersInsideStringsOnly(t *testing.T) {
	raw := "{\n  \"files\": [\n    {\"path\": \"a.go\", \"resolutions\": [\"\tMaxTitleLength = 140\n\tMaxBody = 1\"]}\n  ]\n}"
	if err := json.Unmarshal([]byte(raw), new(any)); err == nil {
		t.Fatal("the raw text should not be valid JSON")
	}
	var doc struct {
		Files []struct {
			Resolutions []string `json:"resolutions"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(Repair(raw)), &doc); err != nil {
		t.Fatalf("repaired text is still invalid: %v", err)
	}
	if got := doc.Files[0].Resolutions[0]; got != "\tMaxTitleLength = 140\n\tMaxBody = 1" {
		t.Fatalf("resolution = %q", got)
	}
}

func TestRepairLeavesValidJSONAlone(t *testing.T) {
	for _, s := range []string{
		`{"a":"line\nbreak","b":["x\ty"],"c":"quote \" inside","d":"back\\slash"}`,
		"{\n  \"a\": 1,\n\t\"b\": [1, 2]\n}",
		`not json at all`,
	} {
		if got := Repair(s); got != s {
			t.Errorf("Repair(%q) = %q", s, got)
		}
	}
}

func TestRepairHandlesOtherControlCharactersAndCarriageReturns(t *testing.T) {
	got := Repair("{\"a\":\"x\x01y\rz\"}")
	var v map[string]string
	if err := json.Unmarshal([]byte(got), &v); err != nil || v["a"] != "x\x01y\rz" {
		t.Fatalf("Repair = %q (%v), decoded %q", got, err, v["a"])
	}
}
