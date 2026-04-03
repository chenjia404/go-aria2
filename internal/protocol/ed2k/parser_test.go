package ed2k

import "testing"

func TestParseLink(t *testing.T) {
	item, err := parseLink("ed2k://|file|demo.iso|12345|abcdef1234567890abcdef1234567890|h=AICHVALUE|s=1.2.3.4:4662|/")
	if err != nil {
		t.Fatalf("parseLink returned error: %v", err)
	}

	if item.Name != "demo.iso" || item.Size != 12345 {
		t.Fatalf("unexpected ed2k link: %+v", item)
	}
	if item.Hash != "abcdef1234567890abcdef1234567890" || item.AICH != "AICHVALUE" {
		t.Fatalf("unexpected ed2k hashes: %+v", item)
	}
	if len(item.Sources) != 1 || item.Sources[0] != "1.2.3.4:4662" {
		t.Fatalf("unexpected ed2k sources: %+v", item.Sources)
	}
}
