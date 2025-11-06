package client

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"testing"
)

func TestProgressReader_EmitsJSONLines(t *testing.T) {
	src := bytes.NewBuffer(make([]byte, 2048))
	buf := new(bytes.Buffer)
	pr := &progressReader{reader: src, writer: buf, fileName: "file.bin", totalSize: 2048}

	r := make([]byte, 512)
	for {
		_, err := pr.Read(r)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read: %v", err)
		}
	}

	scanner := bufio.NewScanner(bytes.NewReader(buf.Bytes()))
	count := 0
	for scanner.Scan() {
		count++
		var u ProgressUpdate
		if err := json.Unmarshal(scanner.Bytes(), &u); err != nil {
			t.Fatalf("bad json: %v", err)
		}
		if u.FileName != "file.bin" || u.Type != "file_progress" {
			t.Fatalf("unexpected update: %+v", u)
		}
	}
	if count == 0 {
		t.Fatalf("expected progress updates")
	}
}
