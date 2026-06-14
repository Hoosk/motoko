package patch

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

func parseUnifiedPatch(input string) (*unifiedPatch, error) {
	lines := strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "--- ") {
			start = i
			break
		}
	}
	if start == -1 || start+1 >= len(lines) || !strings.HasPrefix(lines[start+1], "+++ ") {
		return nil, fmt.Errorf("unified diff invalido; faltan cabeceras ---/+++")
	}
	patch := &unifiedPatch{
		OldPath: normalizeDiffPath(strings.TrimSpace(strings.TrimPrefix(lines[start], "--- "))),
		NewPath: normalizeDiffPath(strings.TrimSpace(strings.TrimPrefix(lines[start+1], "+++ "))),
	}
	for i := start + 2; i < len(lines); {
		line := lines[i]
		if line == "" {
			i++
			continue
		}
		if !strings.HasPrefix(line, "@@ ") {
			return nil, fmt.Errorf("unified diff invalido; hunk esperado y se encontro: %s", line)
		}
		hunk, next, err := parseUnifiedHunk(lines, i)
		if err != nil {
			return nil, err
		}
		patch.Hunks = append(patch.Hunks, hunk)
		i = next
	}
	if len(patch.Hunks) == 0 {
		return nil, fmt.Errorf("unified diff invalido; no contiene hunks")
	}
	return patch, nil
}

func parseUnifiedHunk(lines []string, start int) (unifiedHunk, int, error) {
	match := unifiedHunkHeaderPattern.FindStringSubmatch(lines[start])
	if match == nil {
		return unifiedHunk{}, 0, fmt.Errorf("cabecera de hunk invalida: %s", lines[start])
	}
	hunk := unifiedHunk{
		OldStart: mustParsePatchNumber(match[1]),
		OldCount: parsePatchCount(match[2]),
		NewStart: mustParsePatchNumber(match[3]),
		NewCount: parsePatchCount(match[4]),
	}
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "@@ ") {
			return hunk, i, nil
		}
		if strings.HasPrefix(line, "\\ No newline at end of file") {
			if len(hunk.Lines) == 0 {
				return unifiedHunk{}, 0, fmt.Errorf("marcador de newline invalido en hunk")
			}
			hunk.Lines[len(hunk.Lines)-1].NoNewline = true
			continue
		}
		if line == "" {
			hunk.Lines = append(hunk.Lines, unifiedHunkLine{Kind: ' ', Text: ""})
			continue
		}
		kind := line[0]
		if kind != ' ' && kind != '+' && kind != '-' {
			return unifiedHunk{}, 0, fmt.Errorf("linea de hunk invalida: %s", line)
		}
		hunk.Lines = append(hunk.Lines, unifiedHunkLine{Kind: kind, Text: line[1:]})
	}
	return hunk, len(lines), nil
}

func applyUnifiedPatch(current string, patch *unifiedPatch) (string, error) {
	lines := splitPatchedLines(current)
	result := make([]patchedLine, 0, len(lines))
	pos := 0
	for _, hunk := range patch.Hunks {
		target := hunk.targetIndex()
		if target < pos || target > len(lines) {
			return "", fmt.Errorf("no se puede aplicar el hunk en la linea %d", hunk.OldStart)
		}
		result = append(result, lines[pos:target]...)
		pos = target
		for _, line := range hunk.Lines {
			switch line.Kind {
			case ' ':
				if pos >= len(lines) || lines[pos].Text != line.Text {
					return "", fmt.Errorf("el contexto del hunk no coincide en la linea %d", pos+1)
				}
				result = append(result, lines[pos])
				pos++
			case '-':
				if pos >= len(lines) || lines[pos].Text != line.Text {
					return "", fmt.Errorf("la eliminacion del hunk no coincide en la linea %d", pos+1)
				}
				pos++
			case '+':
				result = append(result, patchedLine{Text: line.Text, NoNewline: line.NoNewline})
			}
		}
	}
	result = append(result, lines[pos:]...)
	return joinPatchedLines(result), nil
}

func (p *unifiedPatch) targetPath() (string, error) {
	oldPath := normalizeDiffPath(p.OldPath)
	newPath := normalizeDiffPath(p.NewPath)
	if newPath == devNull {
		return "", fmt.Errorf("el unified diff no soporta borrado de archivos")
	}
	if oldPath != "" && oldPath != devNull && newPath != "" && oldPath != newPath {
		return "", fmt.Errorf("el unified diff no soporta renombrados; old=%s new=%s", oldPath, newPath)
	}
	if newPath != "" {
		return newPath, nil
	}
	if oldPath != "" && oldPath != devNull {
		return oldPath, nil
	}
	return "", fmt.Errorf("no se pudo resolver la ruta objetivo del unified diff")
}

func (p *unifiedPatch) Render() string {
	var lines []string
	lines = append(lines, "--- "+p.OldPath, "+++ "+p.NewPath)
	for _, hunk := range p.Hunks {
		lines = append(lines, hunk.header())
		for _, line := range hunk.Lines {
			lines = append(lines, string(line.Kind)+line.Text)
			if line.NoNewline {
				lines = append(lines, `\ No newline at end of file`)
			}
		}
	}
	return strings.Join(lines, "\n")
}

func (h unifiedHunk) targetIndex() int {
	if h.OldStart <= 0 {
		return 0
	}
	return h.OldStart - 1
}

func (h unifiedHunk) header() string {
	return fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldCount, h.NewStart, h.NewCount)
}

func splitPatchedLines(content string) []patchedLine {
	if content == "" {
		return nil
	}
	trailingNewline := strings.HasSuffix(content, "\n")
	raw := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	lines := make([]patchedLine, 0, len(raw))
	for i, line := range raw {
		patched := patchedLine{Text: line}
		if i == len(raw)-1 && !trailingNewline {
			patched.NoNewline = true
		}
		lines = append(lines, patched)
	}
	return lines
}

func joinPatchedLines(lines []patchedLine) string {
	if len(lines) == 0 {
		return ""
	}
	var builder strings.Builder
	for i, line := range lines {
		builder.WriteString(line.Text)
		if i < len(lines)-1 || !line.NoNewline {
			builder.WriteByte('\n')
		}
	}
	return builder.String()
}

func normalizeDiffPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = strings.TrimPrefix(path, "a/")
	path = strings.TrimPrefix(path, "b/")
	return filepath.ToSlash(path)
}

func mustParsePatchNumber(raw string) int {
	value, _ := strconv.Atoi(raw)
	return value
}

func parsePatchCount(raw string) int {
	if raw == "" {
		return 1
	}
	return mustParsePatchNumber(raw)
}
