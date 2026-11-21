package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type toolField struct {
	Kind     string
	Required bool
}

// readMCPToolContractsAt parses TestDesk's MCP tool registration files (such as
// internal/mcp/server.go and internal/mcp/v1.go) and extracts every tool's exact
// inputSchema fields and types from map literals and helper calls.
func readMCPToolContractsAt(repositoryRoot, sourceSpec string, expectedTools int, toolPrefix string) (map[string]map[string]toolField, error) {
	sources := splitSourceList(sourceSpec)
	if len(sources) == 0 {
		return nil, fmt.Errorf("MCP registration source list is empty")
	}
	funcs := map[string]*ast.FuncDecl{}
	parsedFiles := make([]*ast.File, 0, len(sources))
	fset := token.NewFileSet()
	for _, rel := range sources {
		abs := filepath.Join(repositoryRoot, filepath.FromSlash(rel))
		f, err := parser.ParseFile(fset, abs, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("parse MCP source %s: %w", rel, err)
		}
		parsedFiles = append(parsedFiles, f)
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				funcs[fn.Name.Name] = fn
			}
		}
	}

	tools := map[string]map[string]toolField{}
	for _, f := range parsedFiles {
		ast.Inspect(f, func(node ast.Node) bool {
			lit, ok := node.(*ast.CompositeLit)
			if !ok {
				return true
			}
			fields, name, ok := extractToolLiteral(lit, funcs)
			if !ok {
				return true
			}
			if !strings.HasPrefix(name, toolPrefix) {
				return true
			}
			tools[name] = fields
			return true
		})
	}
	if len(tools) != expectedTools {
		return nil, fmt.Errorf("found %d MCP tool registrations with prefix %q, expected %d", len(tools), toolPrefix, expectedTools)
	}
	return tools, nil
}

func extractToolLiteral(lit *ast.CompositeLit, funcs map[string]*ast.FuncDecl) (map[string]toolField, string, bool) {
	var name string
	var schemaExpr ast.Expr
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		keyLit, ok := kv.Key.(*ast.BasicLit)
		if !ok || keyLit.Kind != token.STRING {
			continue
		}
		k, err := strconv.Unquote(keyLit.Value)
		if err != nil {
			continue
		}
		switch k {
		case "name":
			if valLit, ok := kv.Value.(*ast.BasicLit); ok && valLit.Kind == token.STRING {
				name, _ = strconv.Unquote(valLit.Value)
			}
		case "inputSchema":
			schemaExpr = kv.Value
		}
	}
	if name == "" || schemaExpr == nil {
		return nil, "", false
	}
	fields := map[string]toolField{}
	extractSchemaExpr(schemaExpr, fields, funcs)
	return fields, name, true
}

func extractSchemaExpr(expr ast.Expr, fields map[string]toolField, funcs map[string]*ast.FuncDecl) {
	if lit, ok := expr.(*ast.CompositeLit); ok {
		_ = extractMapFields(lit, fields)
		return
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return
	}
	ident, ok := call.Fun.(*ast.Ident)
	if !ok {
		return
	}
	switch ident.Name {
	case "projectSchema":
		fields["project"] = toolField{Kind: "string", Required: false}
		if len(call.Args) == 1 {
			extractSchemaExpr(call.Args[0], fields, funcs)
		}
	case "objectSchema":
		if len(call.Args) == 1 {
			extractSchemaExpr(call.Args[0], fields, funcs)
		}
	case "runSchema":
		if fn := funcs["runSchema"]; fn != nil && fn.Body != nil {
			for _, stmt := range fn.Body.List {
				if ret, ok := stmt.(*ast.ReturnStmt); ok && len(ret.Results) == 1 {
					extractSchemaExpr(ret.Results[0], fields, funcs)
				}
			}
		}
	}
}

func extractMapFields(expr ast.Expr, fields map[string]toolField) bool {
	lit, ok := expr.(*ast.CompositeLit)
	if !ok {
		return false
	}
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		keyLit, ok := kv.Key.(*ast.BasicLit)
		if !ok || keyLit.Kind != token.STRING {
			continue
		}
		key, err := strconv.Unquote(keyLit.Value)
		if err != nil || key == "" {
			continue
		}
		kind := schemaCallKind(kv.Value)
		if kind != "unknown" {
			fields[key] = toolField{Kind: kind, Required: false}
		}
	}
	return true
}

func schemaCallKind(expr ast.Expr) string {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return "unknown"
	}
	ident, ok := call.Fun.(*ast.Ident)
	if !ok {
		return "unknown"
	}
	switch ident.Name {
	case "stringSchema", "enumSchema":
		return "string"
	case "numberSchema":
		return "number"
	case "boolSchema":
		return "boolean"
	case "stringArraySchema":
		return "array"
	default:
		return "unknown"
	}
}

func validateContractExamples(filename string, tools map[string]map[string]toolField) error {
	body, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read MCP contract examples: %w", err)
	}
	examples := map[string]map[string]json.RawMessage{}
	if err = decodeStrict(body, &examples); err != nil {
		return fmt.Errorf("invalid MCP contract examples: %w", err)
	}
	if len(examples) != len(tools) {
		return fmt.Errorf("MCP contract examples cover %d tools, expected %d", len(examples), len(tools))
	}
	for toolName, fields := range examples {
		contract, found := tools[toolName]
		if !found {
			return fmt.Errorf("contract example references unregistered MCP tool %q", toolName)
		}
		for name, field := range contract {
			if field.Required {
				if _, found = fields[name]; !found {
					return fmt.Errorf("contract example for %s omits required field %q", toolName, name)
				}
			}
		}
		for name, raw := range fields {
			field, found := contract[name]
			if !found {
				return fmt.Errorf("contract example for %s contains unsupported argument %q", toolName, name)
			}
			var value any
			if err = json.Unmarshal(raw, &value); err != nil {
				return fmt.Errorf("contract example %s.%s is invalid JSON: %w", toolName, name, err)
			}
			if !jsonValueMatchesKind(value, field.Kind) {
				return fmt.Errorf("contract example %s.%s has the wrong JSON type; expected %s", toolName, name, field.Kind)
			}
		}
	}
	return nil
}

func jsonValueMatchesKind(value any, kind string) bool {
	switch kind {
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		_, ok := value.(float64)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	default:
		return false
	}
}

func readCLICommandsAt(filename, helpConstant, cliName string) (map[string]bool, error) {
	file, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse CLI help: %w", err)
	}
	help := ""
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}
		for _, specification := range general.Specs {
			value, ok := specification.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || value.Names[0].Name != helpConstant || len(value.Values) != 1 {
				continue
			}
			text, ok := evalStringExpr(value.Values[0], file)
			if ok {
				help = text
			}
		}
	}
	if help == "" {
		return nil, fmt.Errorf("CLI help constant %s was not found", helpConstant)
	}
	commands := map[string]bool{"help": true}
	for _, match := range cliCommandReferencePatternFor(cliName).FindAllStringSubmatch(help, -1) {
		if len(match) == 2 {
			commands[match[1]] = true
		}
	}
	if len(commands) <= 1 {
		return nil, fmt.Errorf("no CLI commands found in %s help", cliName)
	}
	return commands, nil
}

func evalStringExpr(expr ast.Expr, file *ast.File) (string, bool) {
	switch node := expr.(type) {
	case *ast.BasicLit:
		if node.Kind == token.STRING {
			v, err := strconv.Unquote(node.Value)
			return v, err == nil
		}
	case *ast.Ident:
		for _, decl := range file.Decls {
			g, ok := decl.(*ast.GenDecl)
			if !ok || g.Tok != token.CONST {
				continue
			}
			for _, spec := range g.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if ok && len(vs.Names) == 1 && vs.Names[0].Name == node.Name && len(vs.Values) == 1 {
					return evalStringExpr(vs.Values[0], file)
				}
			}
		}
	case *ast.BinaryExpr:
		if node.Op == token.ADD {
			left, okL := evalStringExpr(node.X, file)
			right, okR := evalStringExpr(node.Y, file)
			if okL && okR {
				return left + right, true
			}
		}
	}
	return "", false
}
