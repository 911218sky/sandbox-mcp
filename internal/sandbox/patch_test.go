package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyPatch_SimpleAddition(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	original := "line1\nline2\nline3\n"
	os.WriteFile(filePath, []byte(original), 0644)

	patch := `--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,4 @@
 line1
+line1.5
 line2
 line3
`
	err := ApplyPatch(filePath, patch)
	if err != nil {
		t.Fatalf("ApplyPatch failed: %v", err)
	}

	content, _ := os.ReadFile(filePath)
	expected := "line1\nline1.5\nline2\nline3\n"
	if string(content) != expected {
		t.Errorf("Expected:\n%q\nGot:\n%q", expected, string(content))
	}
}

func TestApplyPatch_SimpleRemoval(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	original := "line1\nline2\nline3\n"
	os.WriteFile(filePath, []byte(original), 0644)

	patch := `--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,2 @@
 line1
-line2
 line3
`
	err := ApplyPatch(filePath, patch)
	if err != nil {
		t.Fatalf("ApplyPatch failed: %v", err)
	}

	content, _ := os.ReadFile(filePath)
	expected := "line1\nline3\n"
	if string(content) != expected {
		t.Errorf("Expected:\n%q\nGot:\n%q", expected, string(content))
	}
}

func TestApplyPatch_ReplaceLine(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	original := "func main() {\n\tfmt.Println(\"hello\")\n}\n"
	os.WriteFile(filePath, []byte(original), 0644)

	patch := `--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,3 @@
 func main() {
-	fmt.Println("hello")
+	fmt.Println("hello world")
 }
`
	err := ApplyPatch(filePath, patch)
	if err != nil {
		t.Fatalf("ApplyPatch failed: %v", err)
	}

	content, _ := os.ReadFile(filePath)
	expected := "func main() {\n\tfmt.Println(\"hello world\")\n}\n"
	if string(content) != expected {
		t.Errorf("Expected:\n%q\nGot:\n%q", expected, string(content))
	}
}

func TestApplyPatch_AddAndReplace(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	original := "func main() {\n\tfmt.Println(\"hello\")\n}\n"
	os.WriteFile(filePath, []byte(original), 0644)

	patch := `--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,4 @@
 func main() {
-	fmt.Println("hello")
+	fmt.Println("hello world")
+	fmt.Println("goodbye")
 }
`
	err := ApplyPatch(filePath, patch)
	if err != nil {
		t.Fatalf("ApplyPatch failed: %v", err)
	}

	content, _ := os.ReadFile(filePath)
	expected := "func main() {\n\tfmt.Println(\"hello world\")\n\tfmt.Println(\"goodbye\")\n}\n"
	if string(content) != expected {
		t.Errorf("Expected:\n%q\nGot:\n%q", expected, string(content))
	}
}

func TestApplyPatch_MultipleHunks(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	original := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"
	os.WriteFile(filePath, []byte(original), 0644)

	patch := `--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,3 @@
 line1
-line2
+line2-modified
 line3
@@ -8,3 +8,3 @@
 line8
-line9
+line9-modified
 line10
`
	err := ApplyPatch(filePath, patch)
	if err != nil {
		t.Fatalf("ApplyPatch failed: %v", err)
	}

	content, _ := os.ReadFile(filePath)
	expected := "line1\nline2-modified\nline3\nline4\nline5\nline6\nline7\nline8\nline9-modified\nline10\n"
	if string(content) != expected {
		t.Errorf("Expected:\n%q\nGot:\n%q", expected, string(content))
	}
}

func TestApplyPatch_NoHeaderLines(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	original := "line1\nline2\nline3\n"
	os.WriteFile(filePath, []byte(original), 0644)

	patch := `@@ -1,3 +1,3 @@
 line1
-line2
+line2-new
 line3
`
	err := ApplyPatch(filePath, patch)
	if err != nil {
		t.Fatalf("ApplyPatch failed: %v", err)
	}

	content, _ := os.ReadFile(filePath)
	expected := "line1\nline2-new\nline3\n"
	if string(content) != expected {
		t.Errorf("Expected:\n%q\nGot:\n%q", expected, string(content))
	}
}

func TestApplyPatch_NonExistentFile(t *testing.T) {
	err := ApplyPatch("/nonexistent/path/file.txt", `@@ -1,3 +1,3 @@`)
	if err == nil {
		t.Fatal("Expected error for non-existent file")
	}
}

func TestApplyPatch_InvalidHunkHeader(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	os.WriteFile(filePath, []byte("line1\nline2\n"), 0644)

	patch := `--- a/test.txt
+++ b/test.txt
@@ invalid header @@
 line1
`
	err := ApplyPatch(filePath, patch)
	if err == nil {
		t.Fatal("Expected error for invalid hunk header")
	}
}

func TestParseHunkHeader(t *testing.T) {
	tests := []struct {
		input    string
		oldStart int
		oldCount int
		newStart int
		newCount int
	}{
		{"@@ -1,3 +1,4 @@", 1, 3, 1, 4},
		{"@@ -10,5 +10,6 @@", 10, 5, 10, 6},
		{"@@ -1 +1 @@", 1, 1, 1, 1},
		{"@@ -20,100 +20,200 @@ func main()", 20, 100, 20, 200},
	}

	for _, tc := range tests {
		oldStart, oldCount, newStart, newCount, err := parseHunkHeader(tc.input)
		if err != nil {
			t.Errorf("For %q: unexpected error: %v", tc.input, err)
			continue
		}
		if oldStart != tc.oldStart || oldCount != tc.oldCount {
			t.Errorf("For %q: got old=%d,%d, want old=%d,%d", tc.input, oldStart, oldCount, tc.oldStart, tc.oldCount)
		}
		if newStart != tc.newStart || newCount != tc.newCount {
			t.Errorf("For %q: got new=%d,%d, want new=%d,%d", tc.input, newStart, newCount, tc.newStart, tc.newCount)
		}
	}
}

func TestParseHunkHeader_Invalid(t *testing.T) {
	invalids := []string{
		"@@",
		"@@ @@",
		"hello",
	}

	for _, input := range invalids {
		_, _, _, _, err := parseHunkHeader(input)
		if err == nil {
			t.Errorf("Expected error for input %q, got nil", input)
		}
	}
}

func TestGenerateDiff(t *testing.T) {
	original := "line1\nline2\nline3\n"
	modified := "line1\nline2-mod\nline3\n"

	diff := GenerateDiff(original, modified)
	if diff == "" {
		t.Fatal("GenerateDiff should return non-empty string")
	}

	if !strings.Contains(diff, "@@") {
		t.Error("Diff should contain hunk headers @@")
	}
	if !strings.Contains(diff, "---") {
		t.Error("Diff should contain --- header")
	}
	if !strings.Contains(diff, "+++") {
		t.Error("Diff should contain +++ header")
	}
	if !strings.Contains(diff, "-line2") {
		t.Error("Diff should contain removed line")
	}
	if !strings.Contains(diff, "+line2-mod") {
		t.Error("Diff should contain added line")
	}
}

func TestApplyPatch_RewriteEntireFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "test.txt")
	original := "line1\nline2\nline3\n"
	os.WriteFile(filePath, []byte(original), 0644)

	patch := `--- a/test.txt
+++ b/test.txt
@@ -1,3 +1,3 @@
-line1
+new1
-line2
+new2
-line3
+new3
`
	err := ApplyPatch(filePath, patch)
	if err != nil {
		t.Fatalf("ApplyPatch failed: %v", err)
	}

	content, _ := os.ReadFile(filePath)
	expected := "new1\nnew2\nnew3\n"
	if string(content) != expected {
		t.Errorf("Expected:\n%q\nGot:\n%q", expected, string(content))
	}
}
