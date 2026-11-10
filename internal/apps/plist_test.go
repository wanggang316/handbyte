package apps

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const samplePlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleIdentifier</key>
	<string>com.example.Note</string>
	<key>CFBundleName</key>
	<string>Note 書</string>
	<key>LSRequiresNativeExecution</key>
	<true/>
	<key>CFBundleDocumentTypes</key>
	<array><dict><key>CFBundleTypeName</key><string>Inner</string></dict></array>
	<key>CFBundleShortVersionString</key>
	<string>2.1</string>
</dict>
</plist>`

func TestReadInfoInXMLAndBinaryForm(t *testing.T) {
	dir := t.TempDir()
	xmlPath := filepath.Join(dir, "Info.plist")
	if err := os.WriteFile(xmlPath, []byte(samplePlist), 0o600); err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(dir, "Binary.plist")
	if out, err := exec.Command("plutil", "-convert", "binary1", "-o", binPath, xmlPath).CombinedOutput(); err != nil {
		t.Fatalf("plutil: %s %v", out, err)
	}
	for _, path := range []string{xmlPath, binPath} {
		info, err := readInfo(path)
		if err != nil {
			t.Fatalf("%s: %v", filepath.Base(path), err)
		}
		if info["CFBundleIdentifier"] != "com.example.Note" || info["CFBundleName"] != "Note 書" || info["CFBundleShortVersionString"] != "2.1" {
			t.Errorf("%s: %v", filepath.Base(path), info)
		}
		if _, ok := info["CFBundleTypeName"]; ok {
			t.Errorf("%s: a nested key leaked: %v", filepath.Base(path), info)
		}
	}
	if _, err := readBinaryPlist([]byte("bplist00short")); err == nil {
		t.Error("a short binary plist must fail")
	}
}
