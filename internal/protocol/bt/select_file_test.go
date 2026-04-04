package bt

import (
	"reflect"
	"testing"
)

func TestParseAria2SelectFile_All(t *testing.T) {
	all, set, err := parseAria2SelectFile("")
	if err != nil || !all || set != nil {
		t.Fatalf("empty: all=%v set=%v err=%v", all, set, err)
	}
	all, set, err = parseAria2SelectFile("  \t  ")
	if err != nil || !all {
		t.Fatalf("whitespace: all=%v err=%v", all, err)
	}
}

func TestParseAria2SelectFile_Indices(t *testing.T) {
	all, set, err := parseAria2SelectFile("3030-3042,6")
	if err != nil {
		t.Fatal(err)
	}
	if all {
		t.Fatal("expected partial set")
	}
	want := map[int]struct{}{}
	for i := 3030; i <= 3042; i++ {
		want[i] = struct{}{}
	}
	want[6] = struct{}{}
	if !reflect.DeepEqual(set, want) {
		t.Fatalf("got %v want %v", set, want)
	}
}

func TestParseAria2SelectFile_Errors(t *testing.T) {
	for _, s := range []string{"0", "1-0", "a", "1-2-3"} {
		if _, _, err := parseAria2SelectFile(s); err == nil {
			t.Fatalf("expected error for %q", s)
		}
	}
}
