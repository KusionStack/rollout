package utils

import "testing"

func TestAbbreviate(t *testing.T) {
	if len(Abbreviate("", 0)) > 0 {
		t.Errorf("len expect 0")
	}

	if len(Abbreviate("abc", 0)) > 0 {
		t.Errorf("len expect 0")
	}

	str := Abbreviate("abc", 1)
	if str != "a..." {
		t.Errorf("len expect a... but got %s", str)
	}
}
