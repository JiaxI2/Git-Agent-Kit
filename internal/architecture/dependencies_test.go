package architecture

import (
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/JiaxI2/git-isolated-agent-kit/internal/domain"
)

const modulePath = "github.com/JiaxI2/git-isolated-agent-kit"

func TestDomainImportsOnlyStandardLibrary(t *testing.T) {
	root := filepath.Join("..", "domain")
	for _, path := range packageImports(t, root) {
		if strings.HasPrefix(path, modulePath+"/") {
			t.Fatalf("domain imports project package %q", path)
		}
		pkg, err := build.Default.Import(path, root, build.FindOnly)
		if err != nil || !pkg.Goroot {
			t.Fatalf("domain import %q is not in the Go standard library", path)
		}
	}
}

func TestApplicationImportsOnlyDomain(t *testing.T) {
	assertProjectImports(t, filepath.Join("..", "app"), map[string]bool{
		modulePath + "/internal/domain": true,
	})
}

func TestInterfaceAdaptersDependInward(t *testing.T) {
	allowed := map[string]bool{
		modulePath + "/internal/app":    true,
		modulePath + "/internal/domain": true,
	}
	assertProjectImports(t, filepath.Join("..", "adapters", "cli"), allowed)
	assertProjectImports(t, filepath.Join("..", "adapters", "mcp"), allowed)
	assertProjectImports(t, filepath.Join("..", "..", "pkg", "sdk"), allowed)
}

func TestSDKExportedAPIContainsNoInternalTypes(t *testing.T) {
	root := filepath.Join("..", "..", "pkg", "sdk")
	set := token.NewFileSet()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		file, err := parser.ParseFile(set, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		imports := importAliases(file)
		check := func(node ast.Node) {
			ast.Inspect(node, func(candidate ast.Node) bool {
				selector, ok := candidate.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				identifier, ok := selector.X.(*ast.Ident)
				if ok && strings.Contains(imports[identifier.Name], "/internal/") {
					t.Errorf("%s exports internal type %s.%s from %s", entry.Name(), identifier.Name, selector.Sel.Name, imports[identifier.Name])
				}
				return true
			})
		}
		for _, declaration := range file.Decls {
			switch value := declaration.(type) {
			case *ast.FuncDecl:
				if value.Name.IsExported() && receiverIsExported(value.Recv) {
					check(value.Type)
				}
			case *ast.GenDecl:
				for _, specification := range value.Specs {
					typeSpec, ok := specification.(*ast.TypeSpec)
					if ok && typeSpec.Name.IsExported() {
						checkExportedType(typeSpec.Type, check)
					}
				}
			}
		}
	}
}

func TestCoreWorkflowTransitions(t *testing.T) {
	states := []domain.TaskState{
		domain.TaskReady, domain.TaskClaimed, domain.TaskInProgress, domain.TaskValidating,
		domain.TaskReview, domain.TaskDone, domain.TaskBlocked,
	}
	for _, state := range states {
		if strings.TrimSpace(string(state)) == "" {
			t.Fatal("empty workflow state")
		}
	}
	legal := [][2]domain.TaskState{
		{domain.TaskReady, domain.TaskClaimed},
		{domain.TaskClaimed, domain.TaskInProgress},
		{domain.TaskInProgress, domain.TaskValidating},
		{domain.TaskValidating, domain.TaskReview},
		{domain.TaskReview, domain.TaskDone},
	}
	for _, edge := range legal {
		if !domain.CanTransition(edge[0], edge[1]) {
			t.Errorf("required transition %s -> %s is missing", edge[0], edge[1])
		}
	}
	if domain.CanTransition(domain.TaskReady, domain.TaskDone) {
		t.Fatal("ready task can bypass governance and become done")
	}
}

func TestCIMatrixCompilesNewPackagesOnSupportedPlatforms(t *testing.T) {
	path := filepath.Join("..", "..", ".github", "workflows", "ci.yml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, required := range []string{"ubuntu-latest", "windows-latest", "go test ./...", "go vet ./..."} {
		if !strings.Contains(text, required) {
			t.Errorf("CI workflow does not contain %q", required)
		}
	}
}

func TestArchitectureEvolutionLabelsStayOutOfCodeAndPaths(t *testing.T) {
	root := filepath.Join("..", "..")
	versionedSDK := regexp.MustCompile(`(?i)(^|/)pkg/sdk/v[0-9]+(/|$)`)
	versionedDocument := regexp.MustCompile(`(?i)(^|/)(architecture|migration)_v[0-9]+`)
	architectureLabel := regexp.MustCompile(strings.Join([]string{`(?i)architecture\s+`, `v[0-9]+`}, ""))
	formatLabel := strings.Join([]string{"Plan", "Format", "Version"}, "")
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		normalized := filepath.ToSlash(relative)
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			if versionedSDK.MatchString(normalized) {
				t.Errorf("architecture version is encoded in directory %s", normalized)
			}
			return nil
		}
		if versionedDocument.MatchString(normalized) {
			t.Errorf("architecture version is encoded in file path %s", normalized)
		}
		if filepath.Ext(path) != ".go" && filepath.Ext(path) != ".md" {
			return nil
		}
		if normalized == "README.md" || normalized == "CHANGELOG.md" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if architectureLabel.Match(content) || strings.Contains(string(content), formatLabel) {
			t.Errorf("architecture version is encoded in %s", normalized)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertProjectImports(t *testing.T, root string, allowed map[string]bool) {
	t.Helper()
	for _, path := range packageImports(t, root) {
		if strings.HasPrefix(path, modulePath+"/") && !allowed[path] {
			t.Errorf("%s has forbidden project dependency %q", root, path)
		}
	}
}

func packageImports(t *testing.T, root string) []string {
	t.Helper()
	set := token.NewFileSet()
	var imports []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(set, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, item := range file.Imports {
			value, err := strconv.Unquote(item.Path.Value)
			if err != nil {
				return err
			}
			imports = append(imports, value)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return imports
}

func importAliases(file *ast.File) map[string]string {
	aliases := map[string]string{}
	for _, item := range file.Imports {
		path, err := strconv.Unquote(item.Path.Value)
		if err != nil {
			continue
		}
		name := filepath.Base(path)
		if item.Name != nil {
			name = item.Name.Name
		}
		aliases[name] = path
	}
	return aliases
}

func receiverIsExported(receiver *ast.FieldList) bool {
	if receiver == nil {
		return true
	}
	if len(receiver.List) != 1 {
		return false
	}
	typeExpression := receiver.List[0].Type
	if pointer, ok := typeExpression.(*ast.StarExpr); ok {
		typeExpression = pointer.X
	}
	identifier, ok := typeExpression.(*ast.Ident)
	return ok && identifier.IsExported()
}

func checkExportedType(node ast.Expr, check func(ast.Node)) {
	switch value := node.(type) {
	case *ast.StructType:
		for _, field := range value.Fields.List {
			if len(field.Names) == 0 {
				check(field.Type)
				continue
			}
			for _, name := range field.Names {
				if name.IsExported() {
					check(field.Type)
					break
				}
			}
		}
	case *ast.InterfaceType:
		for _, method := range value.Methods.List {
			if len(method.Names) == 0 || method.Names[0].IsExported() {
				check(method.Type)
			}
		}
	default:
		check(node)
	}
}
