package metalink

import "testing"

const sampleMetalink = `<?xml version="1.0" encoding="utf-8"?>
<metalink xmlns="urn:ietf:params:xml:ns:metalink">
  <file name="demo.iso">
    <size>11</size>
    <verification>
      <hash type="sha-256">abcdef</hash>
    </verification>
    <resources>
      <url type="https">http://mirror-a.example/demo.iso</url>
      <url type="https">http://mirror-b.example/demo.iso</url>
    </resources>
  </file>
  <file name="readme.txt">
    <size>5</size>
    <url>magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&amp;dn=readme.txt</url>
  </file>
</metalink>`

func TestParseMetalinkDocument(t *testing.T) {
	t.Parallel()

	files, err := Parse([]byte(sampleMetalink))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if files[0].Name != "demo.iso" || files[0].Size != 11 || files[0].Checksum != "sha-256=abcdef" {
		t.Fatalf("unexpected first file: %#v", files[0])
	}
	if len(files[0].URLs) != 2 {
		t.Fatalf("expected 2 mirrors, got %#v", files[0].URLs)
	}
	if files[1].Name != "readme.txt" || len(files[1].URLs) != 1 {
		t.Fatalf("unexpected second file: %#v", files[1])
	}
}
