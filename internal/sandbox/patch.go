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
	
	// Parse and apply patch
	patchedLines, err := applyUnifiedDiff(originalLines, patch)
	if err != nil {
		return fmt.Errorf("failed to apply patch: %w", err)
	}
	
	// Write patched content
	patchedContent := strings.Join(patchedLines, "\n")
	if err := os.WriteFile(filePath, []byte(patchedContent), 0644); err != nil {
		return fmt.Errorf("failed to write patched file: %w", err)
	}
	
	return nil
}

// applyUnifiedDiff applies a unified diff patch to lines
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
		
		// Parse hunk header @@ -oldStart,oldCount +newStart,newCount @@
		if strings.HasPrefix(line, "@@") {
			oldStart, oldCount, newStart, newCount, err := parseHunkHeader(line)
			if err != nil {
				return nil, fmt.Errorf("invalid hunk header at line %d: %w", patchIdx, err)
			}
			
			// Copy unchanged lines before this hunk
			for originalIdx < oldStart-1 && originalIdx < len(originalLines) {
				result = append(result, originalLines[originalIdx])
				originalIdx++
			}
			
			// Process hunk
			patchIdx++
			hunkOldIdx := 0
			hunkNewIdx := 0
			
			for patchIdx < len(patchLines) {
				patchLine := patchLines[patchIdx]
				
				// Stop at next hunk
				if strings.HasPrefix(patchLine, "@@") {
					break
				}
				
				if strings.HasPrefix(patchLine, "-") {
					// Remove line (skip in original)
					if hunkOldIdx >= oldCount {
						return nil, fmt.Errorf("too many removals in hunk starting at line %d", oldStart)
					}
					originalIdx++
					hunkOldIdx++
				} else if strings.HasPrefix(patchLine, "+") {
					// Add line
					if hunkNewIdx >= newCount {
						return nil, fmt.Errorf("too many additions in hunk starting at line %d", newStart)
					}
					result = append(result, patchLine[1:]) // Remove '+' prefix
					hunkNewIdx++
				} else if strings.HasPrefix(patchLine, " ") || patchLine == "" {
					// Context line (unchanged)
					if originalIdx < len(originalLines) {
						if strings.HasPrefix(patchLine, " ") {
							result = append(result, originalLines[originalIdx])
						} else {
							result = append(result, "") // Empty line
						}
						originalIdx++
						hunkOldIdx++
						hunkNewIdx++
					}
				} else {
					return nil, fmt.Errorf("invalid patch line at line %d: %s", patchIdx, patchLine)
				}
				
				patchIdx++
			}
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
	
	// Simple line-by-line diff (not optimal, but works for small files)
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
