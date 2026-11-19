package jira

import (
	"encoding/json"
	"testing"
)

func TestStatusCategoryIDNumberOrString(t *testing.T) {
	for _, in := range []string{
		`{"name":"Open","statusCategory":{"id":2,"key":"new","name":"To Do"}}`,
		`{"name":"Open","statusCategory":{"id":"2","key":"new","name":"To Do"}}`,
	} {
		var s Status
		if err := json.Unmarshal([]byte(in), &s); err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if s.StatusCategory.ID.String() != "2" {
			t.Errorf("got %q", s.StatusCategory.ID)
		}
	}
}
