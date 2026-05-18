package semantic

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

const (
	maxIndexedFileSize   = 256 * 1024
	maxSymbolsPerFile    = 24
	defaultTopFiles      = 4
	defaultSnippetFiles  = 3
	defaultSnippetBudget = 220
	maxSnippetLines      = 48
	staleAfter           = 30 * time.Second
)

type LineRange struct {
	Start int
	End   int
}

type Symbol struct {
	Name  string
	Kind  string
	Line  int
	Range LineRange
}

type Snippet struct {
	Path      string
	Language  string
	Reason    string
	StartLine int
	EndLine   int
	Content   string
}

func (s Snippet) Descriptor() string {
	return fmt.Sprintf("FILE %s\nLINES %d-%d\nREASON %s\n%s", s.Path, s.StartLine, s.EndLine, s.Reason, s.Content)
}

type FileSummary struct {
	Path     string
	Language string
	Lines    int
	Changed  bool
	Symbols  []Symbol
	Content  []byte
}

func (f FileSummary) Descriptor() string {
	parts := []string{fmt.Sprintf("%s [%s]", f.Path, f.Language)}
	if f.Changed {
		parts = append(parts, "changed")
	}
	if len(f.Symbols) > 0 {
		names := make([]string, 0, min(defaultTopFiles, len(f.Symbols)))
		for i := 0; i < len(f.Symbols) && i < defaultTopFiles; i++ {
			names = append(names, f.Symbols[i].Name)
		}
		parts = append(parts, "symbols: "+strings.Join(names, ", "))
	}
	return strings.Join(parts, " | ")
}

type Snapshot struct {
	GeneratedAt    time.Time
	Root           string
	Directories    []string
	LanguageCounts map[string]int
	ChangedPaths   []string
	Files          []FileSummary
}

func (s Snapshot) Empty() bool {
	return len(s.Files) == 0
}

func (s Snapshot) Summary() string {
	if len(s.Files) == 0 {
		return "indice semantico vacio"
	}
	parts := []string{fmt.Sprintf("files:%d", len(s.Files))}
	if len(s.Directories) > 0 {
		dirs := s.Directories
		if len(dirs) > 6 {
			dirs = dirs[:6]
		}
		parts = append(parts, "dirs:"+strings.Join(dirs, ", "))
	}
	if len(s.LanguageCounts) > 0 {
		langs := make([]string, 0, len(s.LanguageCounts))
		for lang, count := range s.LanguageCounts {
			langs = append(langs, fmt.Sprintf("%s:%d", lang, count))
		}
		sort.Strings(langs)
		parts = append(parts, "langs:"+strings.Join(langs, ", "))
	}
	if len(s.ChangedPaths) > 0 {
		changed := s.ChangedPaths
		if len(changed) > 5 {
			changed = changed[:5]
		}
		parts = append(parts, "changed:"+strings.Join(changed, ", "))
	}
	return strings.Join(parts, " | ")
}

func (s Snapshot) RelevantFiles(prompt string, limit int) []FileSummary {
	return s.relevantFilesForTokens(promptTokens(prompt), limit)
}

func (s Snapshot) RelevantSnippets(prompt string, fileLimit, lineBudget int) []Snippet {
	if lineBudget <= 0 {
		lineBudget = defaultSnippetBudget
	}
	tokens := promptTokens(prompt)
	files := s.relevantFilesForTokens(tokens, fileLimit)
	result := make([]Snippet, 0, len(files))
	remaining := lineBudget
	for _, file := range files {
		if remaining < 12 {
			break
		}
		snippet, ok := bestSnippetForFile(file, tokens, remaining)
		if !ok {
			continue
		}
		result = append(result, snippet)
		remaining -= snippet.EndLine - snippet.StartLine + 1
	}
	return result
}

type scoredFile struct {
	file  FileSummary
	score int
}

func sortScoredFiles(ranked []scoredFile) {
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score == ranked[j].score {
			return ranked[i].file.Path < ranked[j].file.Path
		}
		return ranked[i].score > ranked[j].score
	})
}

func (s Snapshot) relevantFilesForTokens(tokens []string, limit int) []FileSummary {
	if limit <= 0 {
		limit = defaultTopFiles
	}
	var ranked []scoredFile
	for _, file := range s.Files {
		score := scoreFile(file, tokens)
		if score > 0 {
			ranked = append(ranked, scoredFile{file: file, score: score})
		}
	}
	sortScoredFiles(ranked)
	result := make([]FileSummary, 0, limit)
	seen := make(map[string]struct{})
	for _, item := range ranked {
		if len(result) >= limit {
			break
		}
		seen[item.file.Path] = struct{}{}
		result = append(result, item.file)
	}
	if len(result) < limit {
		fallbacks := s.fallbackFiles(limit - len(result))
		for _, file := range fallbacks {
			if _, ok := seen[file.Path]; ok {
				continue
			}
			result = append(result, file)
			if len(result) >= limit {
				break
			}
		}
	}
	return result
}

func (s Snapshot) fallbackFiles(limit int) []FileSummary {
	if limit <= 0 {
		return nil
	}
	var ranked []scoredFile
	for _, file := range s.Files {
		score := 0
		if file.Changed {
			score += 100
		}
		score += len(file.Symbols)
		lowerPath := strings.ToLower(file.Path)
		if strings.Contains(lowerPath, "/app/") || strings.Contains(lowerPath, "/ui/") || strings.Contains(lowerPath, "main") || strings.Contains(lowerPath, "runtime") || strings.Contains(lowerPath, "model") {
			score += 8
		}
		if score > 0 {
			ranked = append(ranked, scoredFile{file: file, score: score})
		}
	}
	sortScoredFiles(ranked)
	result := make([]FileSummary, 0, limit)
	for _, item := range ranked {
		result = append(result, item.file)
		if len(result) >= limit {
			break
		}
	}
	return result
}

type Index struct {
	mu       sync.RWMutex
	snapshot Snapshot
	lastErr  error
}

func NewIndex() *Index {
	return &Index{}
}

func (i *Index) Snapshot() Snapshot {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.snapshot
}

func (i *Index) SetSnapshotForTest(snapshot Snapshot) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.snapshot = snapshot
	i.lastErr = nil
}

func (i *Index) Ensure(ctx context.Context) (Snapshot, error) {
	i.mu.RLock()
	snapshot := i.snapshot
	lastErr := i.lastErr
	i.mu.RUnlock()
	if !snapshot.Empty() && time.Since(snapshot.GeneratedAt) < staleAfter {
		return snapshot, lastErr
	}
	return i.Refresh(ctx)
}

func (i *Index) Refresh(ctx context.Context) (Snapshot, error) {
	root, err := os.Getwd()
	if err != nil {
		return Snapshot{}, err
	}
	snapshot, err := BuildSnapshot(ctx, root)
	i.mu.Lock()
	defer i.mu.Unlock()
	if err == nil {
		i.snapshot = snapshot
	}
	if err != nil {
		i.lastErr = err
		return Snapshot{}, err
	}
	i.lastErr = nil
	return snapshot, nil
}

func BuildSnapshot(ctx context.Context, root string) (Snapshot, error) {
	root = filepath.Clean(root)
	changed := changedPaths(ctx, root)
	changedSet := make(map[string]struct{}, len(changed))
	for _, path := range changed {
		changedSet[path] = struct{}{}
	}

	snapshot := Snapshot{
		GeneratedAt:    time.Now(),
		Root:           root,
		LanguageCounts: make(map[string]int),
		ChangedPaths:   changed,
	}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if skipDir(entry.Name(), rel) {
				return filepath.SkipDir
			}
			if depth(rel) <= 2 {
				snapshot.Directories = append(snapshot.Directories, rel)
			}
			return nil
		}
		if !supportedPath(rel) {
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.Size() > maxIndexedFileSize {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lang, language := languageForPath(rel)
		if lang == nil {
			return nil
		}
		fileSummary := FileSummary{
			Path:     rel,
			Language: language,
			Lines:    countLines(content),
			Changed:  hasChanged(changedSet, rel),
			Symbols:  extractSymbols(content, lang, language),
			Content:  content,
		}
		snapshot.Files = append(snapshot.Files, fileSummary)
		snapshot.LanguageCounts[language]++
		return nil
	})
	if err != nil {
		return Snapshot{}, err
	}
	sort.Strings(snapshot.Directories)
	sort.Strings(snapshot.ChangedPaths)
	sort.Slice(snapshot.Files, func(i, j int) bool {
		return snapshot.Files[i].Path < snapshot.Files[j].Path
	})
	return snapshot, nil
}

func changedPaths(ctx context.Context, root string) []string {
	cmd := exec.CommandContext(ctx, "git", "status", "--short")
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var result []string
	for _, line := range lines {
		if len(strings.TrimSpace(line)) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if idx := strings.Index(path, " -> "); idx >= 0 {
			path = strings.TrimSpace(path[idx+4:])
		}
		if path != "" {
			result = append(result, filepath.ToSlash(path))
		}
	}
	return result
}

func skipDir(name, rel string) bool {
	if rel == ".git" || strings.HasPrefix(rel, ".git/") {
		return true
	}
	if name == "node_modules" || name == "dist" || name == "build" || name == "coverage" {
		return true
	}
	return strings.HasPrefix(name, ".")
}

func supportedPath(path string) bool {
	_, language := languageForPath(path)
	return language != ""
}

func languageForPath(path string) (*sitter.Language, string) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return golang.GetLanguage(), "go"
	case ".js", ".jsx":
		return javascript.GetLanguage(), "javascript"
	case ".ts":
		return typescript.GetLanguage(), "typescript"
	case ".tsx":
		return tsx.GetLanguage(), "tsx"
	default:
		return nil, ""
	}
}

func extractSymbols(content []byte, lang *sitter.Language, language string) []Symbol {
	root, _ := sitter.ParseCtx(context.Background(), content, lang)
	if root == nil {
		return nil
	}
	var symbols []Symbol
	seen := make(map[string]struct{})
	walkNamed(root, func(node *sitter.Node) {
		if len(symbols) >= maxSymbolsPerFile {
			return
		}
		symbol, ok := classifySymbol(node, content, language)
		if !ok || symbol.Name == "" {
			return
		}
		key := symbol.Kind + ":" + symbol.Name
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		symbols = append(symbols, symbol)
	})
	return symbols
}

func walkNamed(node *sitter.Node, visit func(*sitter.Node)) {
	if node == nil {
		return
	}
	visit(node)
	for i := 0; i < int(node.NamedChildCount()); i++ {
		walkNamed(node.NamedChild(i), visit)
	}
}

func classifySymbol(node *sitter.Node, content []byte, language string) (Symbol, bool) {
	switch language {
	case "go":
		return classifyGoSymbol(node, content)
	case "javascript", "typescript", "tsx":
		return classifyJSSymbol(node, content)
	default:
		return Symbol{}, false
	}
}

func classifyGoSymbol(node *sitter.Node, content []byte) (Symbol, bool) {
	switch node.Type() {
	case "function_declaration":
		return symbolFromField(node, content, "name", "func")
	case "method_declaration":
		return symbolFromField(node, content, "name", "method")
	case "type_spec":
		return symbolFromField(node, content, "name", "type")
	case "var_spec":
		return symbolFromFirstNamedChild(node, content, "var")
	case "const_spec":
		return symbolFromFirstNamedChild(node, content, "const")
	default:
		return Symbol{}, false
	}
}

func classifyJSSymbol(node *sitter.Node, content []byte) (Symbol, bool) {
	switch node.Type() {
	case "function_declaration":
		return symbolFromField(node, content, "name", "func")
	case "class_declaration":
		return symbolFromField(node, content, "name", "class")
	case "method_definition":
		return symbolFromField(node, content, "name", "method")
	case "interface_declaration":
		return symbolFromField(node, content, "name", "interface")
	case "type_alias_declaration":
		return symbolFromField(node, content, "name", "type")
	case "variable_declarator":
		return symbolFromField(node, content, "name", "var")
	default:
		return Symbol{}, false
	}
}

func createSymbol(node, nameNode *sitter.Node, name, kind string) (Symbol, bool) {
	return Symbol{
		Name: name,
		Kind: kind,
		Line: int(nameNode.StartPoint().Row) + 1,
		Range: LineRange{
			Start: int(node.StartPoint().Row) + 1,
			End:   int(node.EndPoint().Row) + 1,
		},
	}, true
}

func symbolFromField(node *sitter.Node, content []byte, field, kind string) (Symbol, bool) {
	nameNode := node.ChildByFieldName(field)
	if nameNode == nil {
		return Symbol{}, false
	}
	name := strings.TrimSpace(nameNode.Content(content))
	if name == "" {
		return Symbol{}, false
	}
	return createSymbol(node, nameNode, name, kind)
}

func symbolFromFirstNamedChild(node *sitter.Node, content []byte, kind string) (Symbol, bool) {
	for i := 0; i < int(node.NamedChildCount()); i++ {
		child := node.NamedChild(i)
		if child == nil {
			continue
		}
		name := strings.TrimSpace(child.Content(content))
		if name != "" {
			return createSymbol(node, child, name, kind)
		}
	}
	return Symbol{}, false
}

func bestSnippetForFile(file FileSummary, tokens []string, budget int) (Snippet, bool) {
	if len(file.Content) == 0 {
		return Snippet{}, false
	}
	lines := splitLines(file.Content)
	if len(lines) == 0 {
		return Snippet{}, false
	}
	bestSymbol, reason := chooseSymbol(file, tokens)
	var start, end int
	if bestSymbol.Name != "" {
		start, end = boundedRange(bestSymbol.Range.Start, bestSymbol.Range.End, len(lines), budget)
	} else {
		start, end = boundedRange(1, min(len(lines), 20), len(lines), budget)
		reason = "fallback top of file"
	}
	content := strings.Join(lines[start-1:end], "\n")
	return Snippet{
		Path:      file.Path,
		Language:  file.Language,
		Reason:    reason,
		StartLine: start,
		EndLine:   end,
		Content:   content,
	}, true
}

func chooseSymbol(file FileSummary, tokens []string) (Symbol, string) {
	bestScore := -1
	var best Symbol
	reason := "fallback top-level symbol"
	for _, symbol := range file.Symbols {
		score := 0
		name := strings.ToLower(symbol.Name)
		for _, token := range tokens {
			if token == "" {
				continue
			}
			if name == token {
				score += 20
			}
			if strings.Contains(name, token) || strings.Contains(token, name) {
				score += 12
			}
		}
		if file.Changed {
			score += 5
		}
		if score > bestScore {
			bestScore = score
			best = symbol
			if score > 0 {
				reason = "symbol match: " + symbol.Name
			}
		}
	}
	if bestScore < 0 {
		return Symbol{}, ""
	}
	if best.Name == "" {
		return Symbol{}, ""
	}
	if bestScore > 0 && file.Changed {
		reason += " in changed file"
	}
	return best, reason
}

func boundedRange(start, end, totalLines, budget int) (int, int) {
	if start < 1 {
		start = 1
	}
	if end < start {
		end = start
	}
	if totalLines <= 0 {
		return 1, 1
	}
	if budget <= 0 {
		budget = maxSnippetLines
	}
	maxLines := min(maxSnippetLines, budget)
	if maxLines < 8 {
		maxLines = min(8, totalLines)
	}
	length := end - start + 1
	if length > maxLines {
		end = start + maxLines - 1
	}
	if end > totalLines {
		end = totalLines
	}
	if start > totalLines {
		start = max(1, totalLines-maxLines+1)
		end = totalLines
	}
	return start, end
}

func splitLines(content []byte) []string {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func scoreFile(file FileSummary, tokens []string) int {
	path := strings.ToLower(file.Path)
	base := strings.ToLower(strings.TrimSuffix(filepath.Base(file.Path), filepath.Ext(file.Path)))
	score := 0
	if file.Changed {
		score += 35
	}
	for _, token := range tokens {
		if token == "" {
			continue
		}
		if strings.Contains(path, token) {
			score += 10
		}
		if strings.Contains(base, token) {
			score += 6
		}
		for _, symbol := range file.Symbols {
			name := strings.ToLower(symbol.Name)
			if name == token {
				score += 20
				continue
			}
			if strings.Contains(name, token) {
				score += 12
			}
		}
	}
	if len(tokens) == 0 && file.Changed {
		score += 20
	}
	if strings.Contains(path, "_test.go") && containsToken(tokens, "test") {
		score += 5
	}
	return score
}

func promptTokens(prompt string) []string {
	parts := strings.FieldsFunc(strings.ToLower(prompt), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '_' && r != '-' && r != '/'
	})
	stop := map[string]struct{}{
		"que": {}, "del": {}, "para": {}, "con": {}, "this": {}, "that": {}, "sobre": {},
		"quiero": {}, "opinion": {}, "codigo": {}, "code": {}, "file": {}, "archivo": {},
		"the": {}, "and": {}, "una": {}, "por": {}, "favor": {}, "quieres": {},
	}
	seen := make(map[string]struct{})
	var tokens []string
	for _, part := range parts {
		part = strings.Trim(part, "_-/")
		if len(part) < 2 {
			continue
		}
		if _, ok := stop[part]; ok {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		tokens = append(tokens, part)
	}
	return tokens
}

func containsToken(tokens []string, target string) bool {
	for _, token := range tokens {
		if token == target {
			return true
		}
	}
	return false
}

func hasChanged(changed map[string]struct{}, path string) bool {
	_, ok := changed[path]
	return ok
}

func countLines(content []byte) int {
	if len(content) == 0 {
		return 0
	}
	count := 1
	for _, b := range content {
		if b == '\n' {
			count++
		}
	}
	return count
}

func depth(path string) int {
	if path == "" || path == "." {
		return 0
	}
	return strings.Count(path, "/") + 1
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
