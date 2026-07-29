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

func TestParseLinkUsesGoed2kParser(t *testing.T) {
	item, err := parseLink("ed2k://|file|hello%20world.mkv|999|0123456789abcdef0123456789abcdef|/")
	if err != nil {
		t.Fatalf("parseLink returned error: %v", err)
	}
	if item.Name != "hello world.mkv" {
		t.Fatalf("unexpected name: %q", item.Name)
	}
	if item.Size != 999 {
		t.Fatalf("unexpected size: %d", item.Size)
	}
}

func TestParseLinkExtras(t *testing.T) {
	aich, sources := parseLinkExtras("ed2k://|file|x|1|hash|h=foo|s=1.1.1.1:4662|s=2.2.2.2:4662|/")
	if aich != "foo" {
		t.Fatalf("unexpected aich: %q", aich)
	}
	if len(sources) != 2 || sources[0] != "1.1.1.1:4662" || sources[1] != "2.2.2.2:4662" {
		t.Fatalf("unexpected sources: %+v", sources)
	}
}

func TestParseLinkRejectsNonFile(t *testing.T) {
	_, err := parseLink("ed2k://|server|1.2.3.4|4661|/")
	if err == nil {
		t.Fatal("expected error for server link")
	}
}
