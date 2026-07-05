package sandbox

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ApplyPatch applies a unified diff patch to a file
// Returns error if patch cannot be applied
func ApplyPatch(filePath string, patch string) error {
	// Read original file
	originalContent, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file for patching: %w", err)
	}
	
	originalLines := strings.Split(string(originalContent), "\n")
	// Remove trailing empty string if file ends with newline
	if len(originalLines) > 0 && originalLines[len(originalLines)-1] == "" {
		originalLines = originalLines[:len(originalLines)-1]
	}
	
	// Parse and apply patch
	patchedLines, err := applyUnifiedDiff(originalLines, patch)
	if err != nil {
		return fmt.Errorf("failed to apply patch: %w", err)
	}
	
	// Write patched content
	patchedContent := strings.Join(patchedLines, "\n") + "\n"
	if err := os.WriteFile(filePath, []byte(patchedContent), 0644); err != nil {
		return fmt.Errorf("failed to write patched file: %w", err)
	}
	
	return nil
}

// parseHunk extracts old lines (context+removed) and new lines (context+added) from a hunk
// oldCount is the number of old lines expected (from hunk header), used to stop correctly
func parseHunk(patchLines []string, startIdx int, oldCount int) (oldLines, newLines []string, endIdx int) {
	idx := startIdx
	oldSeen := 0
	for idx < len(patchLines) && oldSeen < oldCount {
		line := patchLines[idx]
		if strings.HasPrefix(line, "@@") {
			break
		}
		if strings.HasPrefix(line, "-") {
			oldLines = append(oldLines, line[1:])
			oldSeen++
		} else if strings.HasPrefix(line, "+") {
			newLines = append(newLines, line[1:])
		} else if strings.HasPrefix(line, " ") {
			oldLines = append(oldLines, line[1:])
			newLines = append(newLines, line[1:])
			oldSeen++
		} else if line == "" {
			// Empty line in diff context (represents an empty line in the file)
			oldLines = append(oldLines, "")
			newLines = append(newLines, "")
			oldSeen++
		} else {
			// Unknown prefix, skip
			break
		}
		idx++
	}
	// Skip any remaining "+" lines that are part of this hunk
	for idx < len(patchLines) && strings.HasPrefix(patchLines[idx], "+") {
		newLines = append(newLines, patchLines[idx][1:])
		idx++
	}
	return oldLines, newLines, idx
}

// linesMatchExact checks if two sets of lines match exactly
func linesMatchExact(original []string, start int, pattern []string) bool {
	if start < 0 || start+len(pattern) > len(original) {
		return false
	}
	for i, line := range pattern {
		if original[start+i] != line {
			return false
		}
	}
	return true
}

// linesMatchTrim checks if two sets of lines match ignoring leading/trailing whitespace
func linesMatchTrim(original []string, start int, pattern []string) bool {
	if start < 0 || start+len(pattern) > len(original) {
		return false
	}
	for i, line := range pattern {
		if strings.TrimSpace(original[start+i]) != strings.TrimSpace(line) {
			return false
		}
	}
	return true
}

// findFuzzyMatch searches for oldLines in originalLines starting near targetPos
// Supports: exact match, whitespace-insensitive match, and offset-based fuzzy match
func findFuzzyMatch(originalLines []string, oldLines []string, targetPos int) int {
	if len(oldLines) == 0 {
		return targetPos
	}

	// Pass 1: Exact match at targetPos
	if linesMatchExact(originalLines, targetPos, oldLines) {
		return targetPos
	}

	// Pass 2: Exact match with offset search (±20 lines)
	maxOffset := 20
	for offset := 1; offset <= maxOffset; offset++ {
		if linesMatchExact(originalLines, targetPos+offset, oldLines) {
			return targetPos + offset
		}
		if linesMatchExact(originalLines, targetPos-offset, oldLines) {
			return targetPos - offset
		}
	}

	// Pass 3: Whitespace-insensitive match at targetPos
	if linesMatchTrim(originalLines, targetPos, oldLines) {
		return targetPos
	}

	// Pass 4: Whitespace-insensitive match with offset search
	for offset := 1; offset <= maxOffset; offset++ {
		if linesMatchTrim(originalLines, targetPos+offset, oldLines) {
			return targetPos + offset
		}
		if linesMatchTrim(originalLines, targetPos-offset, oldLines) {
			return targetPos - offset
		}
	}

	return -1
}

// applyUnifiedDiff applies a unified diff patch to lines with fuzzy matching support
func applyUnifiedDiff(originalLines []string, patch string) ([]string, error) {
	result := make([]string, 0, len(originalLines))
	patchLines := strings.Split(patch, "\n")

	originalIdx := 0
	patchIdx := 0

	// Skip header lines (---, +++)
	for patchIdx < len(patchLines) {
		line := patchLines[patchIdx]
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
			patchIdx++
		} else {
			break
		}
	}

	for patchIdx < len(patchLines) {
		line := patchLines[patchIdx]

		if strings.HasPrefix(line, "@@") {
			oldStart, oldCount, _, _, err := parseHunkHeader(line)
			if err != nil {
				return nil, fmt.Errorf("invalid hunk header at line %d: %w", patchIdx, err)
			}

			patchIdx++

			// Parse the hunk into old/new line sets, respecting oldCount
			oldLines, newLines, nextPatchIdx := parseHunk(patchLines, patchIdx, oldCount)
			patchIdx = nextPatchIdx

			// Find where the old lines match in the original (with fuzzy search)
			targetPos := oldStart - 1
			if originalIdx > targetPos {
				targetPos = originalIdx
			}
			matchPos := findFuzzyMatch(originalLines, oldLines, targetPos)
			if matchPos == -1 {
				return nil, fmt.Errorf("could not find matching context near line %d", oldStart)
			}

			// Copy unchanged lines before the match
			for originalIdx < matchPos {
				result = append(result, originalLines[originalIdx])
				originalIdx++
			}

			// Apply the hunk: skip old lines, add new lines
			result = append(result, newLines...)
			originalIdx += len(oldLines)
		} else {
			patchIdx++
		}
	}

	// Copy remaining original lines
	for originalIdx < len(originalLines) {
		result = append(result, originalLines[originalIdx])
		originalIdx++
	}

	return result, nil
}

// parseHunkHeader parses @@ -oldStart,oldCount +newStart,newCount @@
func parseHunkHeader(header string) (oldStart, oldCount, newStart, newCount int, err error) {
	// Remove @@ markers
	header = strings.TrimPrefix(header, "@@")
	header = strings.TrimSuffix(header, "@@")
	header = strings.TrimSpace(header)
	
	parts := strings.Fields(header)
	if len(parts) < 2 {
		return 0, 0, 0, 0, fmt.Errorf("invalid hunk header format")
	}
	
	// Parse -oldStart,oldCount
	oldRange := strings.TrimPrefix(parts[0], "-")
	oldParts := strings.Split(oldRange, ",")
	oldStart, err = strconv.Atoi(oldParts[0])
	if err != nil {
		return 0, 0, 0, 0, err
	}
	if len(oldParts) == 2 {
		oldCount, err = strconv.Atoi(oldParts[1])
		if err != nil {
			return 0, 0, 0, 0, err
		}
	} else {
		oldCount = 1
	}
	
	// Parse +newStart,newCount
	newRange := strings.TrimPrefix(parts[1], "+")
	newParts := strings.Split(newRange, ",")
	newStart, err = strconv.Atoi(newParts[0])
	if err != nil {
		return 0, 0, 0, 0, err
	}
	if len(newParts) == 2 {
		newCount, err = strconv.Atoi(newParts[1])
		if err != nil {
			return 0, 0, 0, 0, err
		}
	} else {
		newCount = 1
	}
	
	return oldStart, oldCount, newStart, newCount, nil
}

// GenerateDiff creates a unified diff between two strings (for testing)
func GenerateDiff(original, modified string) string {
	originalLines := strings.Split(original, "\n")
	modifiedLines := strings.Split(modified, "\n")
	
	var diff strings.Builder
	diff.WriteString("--- original\n")
	diff.WriteString("+++ modified\n")
	
	maxLen := len(originalLines)
	if len(modifiedLines) > maxLen {
		maxLen = len(modifiedLines)
	}
	
	startLine := 1
	var hunk strings.Builder
	changes := 0
	
	for i := 0; i < maxLen; i++ {
		var oldLine, newLine string
		if i < len(originalLines) {
			oldLine = originalLines[i]
		}
		if i < len(modifiedLines) {
			newLine = modifiedLines[i]
		}
		
		if oldLine != newLine {
			if changes == 0 {
				hunk.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", 
					startLine, 3, startLine, 3))
			}
			if i < len(originalLines) {
				hunk.WriteString(fmt.Sprintf("-%s\n", oldLine))
			}
			if i < len(modifiedLines) {
				hunk.WriteString(fmt.Sprintf("+%s\n", newLine))
			}
			changes++
		} else {
			if changes > 0 && changes < 3 {
				hunk.WriteString(fmt.Sprintf(" %s\n", oldLine))
				changes++
			} else if changes >= 3 {
				diff.WriteString(hunk.String())
				hunk.Reset()
				changes = 0
				startLine = i + 1
			}
		}
	}
	
	if hunk.Len() > 0 {
		diff.WriteString(hunk.String())
	}
	
	return diff.String()
}
