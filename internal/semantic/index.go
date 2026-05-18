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
	"github.com/smacker/go-tree-sitter/cpp"
	"github.com/smacker/go-tree-sitter/css"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/html"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/ruby"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
	"github.com/smacker/go-tree-sitter/yaml"
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
	End int
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
	Imports  []string
	Exports  []string
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

func scoreFile(file FileSummary, tokens []string) int {
	score := 0
	lowerPath := strings.ToLower(file.Path)
	for _, token := range tokens {
		if token == "" {
			continue
		}
		if strings.Contains(lowerPath, token) {
			score += 10
		}
		for _, symbol := range file.Symbols {
			if strings.ToLower(symbol.Name) == token {
				score += 25
			} else if strings.Contains(strings.ToLower(symbol.Name), token) {
				score += 15
			}
		}
	}
	if file.Changed {
		score += 30
	}
	return score
}

type Index struct {
	mu           sync.RWMutex
	lastSnapshot *Snapshot
}

func NewIndex() *Index {
	return &Index{}
}

func (idx *Index) Ensure(ctx context.Context) (*Snapshot, error) {
	idx.mu.RLock()
	s := idx.lastSnapshot
	idx.mu.RUnlock()
	if s != nil && time.Since(s.GeneratedAt) < staleAfter {
		return s, nil
	}
	return idx.Refresh(ctx)
}

func (idx *Index) Refresh(ctx context.Context) (*Snapshot, error) {
	return idx.RefreshDir(ctx, "")
}

func (idx *Index) RefreshDir(ctx context.Context, root string) (*Snapshot, error) {
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	snapshot := &Snapshot{
		GeneratedAt:    time.Now(),
		Root:           root,
		LanguageCounts: make(map[string]int),
	}
	changed := findChangedFiles(root)
	snapshot.ChangedPaths = changed
	changedMap := make(map[string]bool)
	for _, p := range changed {
		changedMap[p] = true
	}
	dirsSeen := make(map[string]bool)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" || d.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if !isSupported(rel) {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > maxIndexedFileSize {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lang, langName := languageForPath(rel)
		symbols, imports, exports := extractSymbolsAndDeps(content, lang, langName)
		summary := FileSummary{
			Path:     rel,
			Language: langName,
			Lines:    strings.Count(string(content), "\n") + 1,
			Changed:  changedMap[rel],
			Symbols:  symbols,
			Imports:  imports,
			Exports:  exports,
			Content:  content,
		}
		snapshot.Files = append(snapshot.Files, summary)
		snapshot.LanguageCounts[langName]++
		dir := filepath.Dir(rel)
		if dir != "." && !dirsSeen[dir] {
			dirsSeen[dir] = true
			snapshot.Directories = append(snapshot.Directories, dir)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	idx.mu.Lock()
	idx.lastSnapshot = snapshot
	idx.mu.Unlock()
	return snapshot, nil
}

func findChangedFiles(root string) []string {
	cmd := exec.Command("git", "status", "--short")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var paths []string
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) < 3 {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		paths = append(paths, parts[len(parts)-1])
	}
	return paths
}

func isSupported(path string) bool {
	_, language := languageForPath(path)
	return language != ""
}

func languageForPath(path string) (*sitter.Language, string) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return golang.GetLanguage(), "go"
	case ".py":
		return python.GetLanguage(), "python"
	case ".rs":
		return rust.GetLanguage(), "rust"
	case ".cpp", ".cc", ".cxx", ".hpp", ".h":
		return cpp.GetLanguage(), "cpp"
	case ".rb":
		return ruby.GetLanguage(), "ruby"
	case ".js", ".jsx":
		return javascript.GetLanguage(), "javascript"
	case ".ts":
		return typescript.GetLanguage(), "typescript"
	case ".tsx":
		return tsx.GetLanguage(), "tsx"
	case ".css":
		return css.GetLanguage(), "css"
	case ".html", ".htm":
		return html.GetLanguage(), "html"
	case ".yaml", ".yml":
		return yaml.GetLanguage(), "yaml"
	default:
		return nil, ""
	}
}

func extractSymbolsAndDeps(content []byte, lang *sitter.Language, language string) ([]Symbol, []string, []string) {
	if lang == nil {
		return nil, nil, nil
	}
	root, _ := sitter.ParseCtx(context.Background(), content, lang)
	if root == nil {
		return nil, nil, nil
	}
	var symbols []Symbol
	var imports []string
	var exports []string

	seenSymbols := make(map[string]struct{})
	seenImports := make(map[string]struct{})

	walkNamed(root, func(node *sitter.Node) {
		// Handle Imports
		if isImportNode(node, language) {
			path := extractImportPath(node, content, language)
			if path != "" {
				if _, exists := seenImports[path]; !exists {
					seenImports[path] = struct{}{}
					imports = append(imports, path)
				}
			}
			return
		}

		if len(symbols) >= maxSymbolsPerFile {
			return
		}

		symbol, ok := classifySymbol(node, content, language)
		if ok && symbol.Name != "" {
			key := symbol.Kind + ":" + symbol.Name
			if _, exists := seenSymbols[key]; !exists {
				seenSymbols[key] = struct{}{}
				symbols = append(symbols, symbol)

				// For Go, symbols starting with Uppercase are exports
				if language == "go" && len(symbol.Name) > 0 && unicode.IsUpper(rune(symbol.Name[0])) {
					exports = append(exports, symbol.Name)
				}
				// For JS/TS, if parent is an export statement, it's an export
				if (language == "javascript" || language == "typescript" || language == "tsx") && isChildOfExport(node) {
					exports = append(exports, symbol.Name)
				}
			}
		}
	})
	return symbols, imports, exports
}

func isImportNode(node *sitter.Node, language string) bool {
	t := node.Type()
	switch language {
	case "go":
		return t == "import_spec"
	case "python":
		return t == "import_from_statement" || t == "import_statement"
	case "rust":
		return t == "use_declaration"
	case "cpp":
		return t == "preproc_include"
	case "javascript", "typescript", "tsx":
		return t == "import_statement" || t == "import_declaration"
	}
	return false
}

func isExportNode(node *sitter.Node, language string) bool {
	t := node.Type()
	switch language {
	case "javascript", "typescript", "tsx":
		return t == "export_statement" || t == "export_declaration"
	}
	return false
}

func isChildOfExport(node *sitter.Node) bool {
	p := node.Parent()
	for p != nil {
		t := p.Type()
		if t == "export_statement" || t == "export_declaration" {
			return true
		}
		p = p.Parent()
	}
	return false
}

func extractImportPath(node *sitter.Node, content []byte, language string) string {
	switch language {
	case "go":
		pathNode := node.ChildByFieldName("path")
		if pathNode == nil {
			for i := 0; i < int(node.NamedChildCount()); i++ {
				c := node.NamedChild(i)
				if c.Type() == "interpreted_string_literal" || c.Type() == "raw_string_literal" {
					pathNode = c
					break
				}
			}
		}
		if pathNode != nil {
			return strings.Trim(string(pathNode.Content(content)), "\"")
		}
	case "python":
		// 'import from' or direct 'import'
		nameNode := node.ChildByFieldName("module_name")
		if nameNode == nil {
			nameNode = node.ChildByFieldName("name")
		}
		if nameNode != nil {
			return strings.TrimSpace(string(nameNode.Content(content)))
		}
	case "rust":
		// 'use foo::bar'
		pathNode := node.ChildByFieldName("argument")
		if pathNode != nil {
			return strings.TrimSpace(string(pathNode.Content(content)))
		}
	case "cpp":
		// #include <path> or "path"
		pathNode := node.ChildByFieldName("path")
		if pathNode != nil {
			return strings.Trim(string(pathNode.Content(content)), "<>\"")
		}
	case "javascript", "typescript", "tsx":
		sourceNode := node.ChildByFieldName("source")
		if sourceNode != nil {
			return strings.Trim(string(sourceNode.Content(content)), "'\"")
		}
	}
	return ""
}

func extractSymbols(content []byte, lang *sitter.Language, language string) []Symbol {
	s, _, _ := extractSymbolsAndDeps(content, lang, language)
	return s
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
	case "python":
		return classifyPythonSymbol(node, content)
	case "rust":
		return classifyRustSymbol(node, content)
	case "cpp":
		return classifyCppSymbol(node, content)
	case "ruby":
		return classifyRubySymbol(node, content)
	case "javascript", "typescript", "tsx":
		return classifyJSSymbol(node, content)
	case "css":
		return classifyCssSymbol(node, content)
	case "html":
		return classifyHtmlSymbol(node, content)
	case "yaml":
		return classifyYamlSymbol(node, content)
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

func classifyPythonSymbol(node *sitter.Node, content []byte) (Symbol, bool) {
	switch node.Type() {
	case "function_definition":
		return symbolFromField(node, content, "name", "func")
	case "class_definition":
		return symbolFromField(node, content, "name", "class")
	default:
		return Symbol{}, false
	}
}

func classifyRustSymbol(node *sitter.Node, content []byte) (Symbol, bool) {
	switch node.Type() {
	case "function_item":
		return symbolFromField(node, content, "name", "func")
	case "struct_item":
		return symbolFromField(node, content, "name", "struct")
	case "enum_item":
		return symbolFromField(node, content, "name", "enum")
	case "trait_item":
		return symbolFromField(node, content, "name", "trait")
	case "impl_item":
		// impls usually don't have a simple name field, skip for now or use type name
		return Symbol{}, false
	case "mod_item":
		return symbolFromField(node, content, "name", "mod")
	default:
		return Symbol{}, false
	}
}

func classifyCppSymbol(node *sitter.Node, content []byte) (Symbol, bool) {
	switch node.Type() {
	case "function_definition":
		declarator := node.ChildByFieldName("declarator")
		if declarator != nil {
			// simplified: find the identifier
			return symbolFromFirstNamedChild(declarator, content, "func")
		}
	case "class_specifier":
		return symbolFromField(node, content, "name", "class")
	case "struct_specifier":
		return symbolFromField(node, content, "name", "struct")
	}
	return Symbol{}, false
}

func classifyRubySymbol(node *sitter.Node, content []byte) (Symbol, bool) {
	switch node.Type() {
	case "method":
		return symbolFromField(node, content, "name", "method")
	case "class":
		return symbolFromField(node, content, "name", "class")
	case "module":
		return symbolFromField(node, content, "name", "module")
	}
	return Symbol{}, false
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

func classifyCssSymbol(node *sitter.Node, content []byte) (Symbol, bool) {
	if node.Type() == "rule_set" {
		selectors := node.ChildByFieldName("selectors")
		if selectors != nil {
			return createSymbol(node, selectors, strings.TrimSpace(string(selectors.Content(content))), "rule")
		}
	}
	return Symbol{}, false
}

func classifyHtmlSymbol(node *sitter.Node, content []byte) (Symbol, bool) {
	if node.Type() == "element" {
		startTag := node.Child(0)
		if startTag != nil && startTag.Type() == "start_tag" {
			nameNode := startTag.Child(1) // tag name
			if nameNode != nil {
				return createSymbol(node, nameNode, string(nameNode.Content(content)), "tag")
			}
		}
	}
	return Symbol{}, false
}

func classifyYamlSymbol(node *sitter.Node, content []byte) (Symbol, bool) {
	if node.Type() == "block_mapping_pair" {
		keyNode := node.ChildByFieldName("key")
		if keyNode != nil {
			return createSymbol(node, keyNode, strings.TrimSpace(string(keyNode.Content(content))), "key")
		}
	}
	return Symbol{}, false
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
	if start <= 0 || end > len(lines) || start > end {
		return Snippet{}, false
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
	return best, reason
}

func splitLines(content []byte) []string {
	s := string(content)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(s, "\n")
}

func boundedRange(start, end, totalLines, budget int) (int, int) {
	if budget <= 0 {
		budget = 20
	}
	if budget > maxSnippetLines {
		budget = maxSnippetLines
	}
	lines := end - start + 1
	if lines > budget {
		return start, start + budget - 1
	}
	needed := budget - lines
	before := needed / 2
	after := needed - before
	newStart := max(1, start-before)
	newEnd := min(totalLines, end+after)
	return newStart, newEnd
}

func promptTokens(prompt string) []string {
	var tokens []string
	var current strings.Builder
	for _, r := range prompt {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			current.WriteRune(unicode.ToLower(r))
		} else {
			if current.Len() > 2 {
				tokens = append(tokens, current.String())
			}
			current.Reset()
		}
	}
	if current.Len() > 2 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

func (idx *Index) SetSnapshotForTest(s *Snapshot) {
	idx.mu.Lock()
	idx.lastSnapshot = s
	idx.mu.Unlock()
}

func BuildSnapshot(ctx context.Context, root string) (*Snapshot, error) {
	idx := NewIndex()
	return idx.RefreshDir(ctx, root)
}
