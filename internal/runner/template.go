package runner

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

// TemplateData contains the small, explicit vocabulary accepted in provider
// command templates. This avoids arbitrary shell interpolation.
type TemplateData struct {
	ProjectRoot  string
	Provider     domain.Provider
	Test         *domain.TestCase
	ArtifactPath string
}

var placeholderPattern = regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`)

// RenderCommand expands a command spec into an executable invocation.
func RenderCommand(spec domain.CommandSpec, data TemplateData) (domain.CommandSpec, error) {
	if spec.Exe == "" {
		return spec, fmt.Errorf("command executable is empty")
	}
	for _, arg := range spec.Args {
		if strings.Contains(arg, "{{test.goPackage}}") && (data.Test == nil || data.Test.GoPackage == "") {
			return spec, fmt.Errorf("Go test package is missing; rediscover tests before running this selection")
		}
	}
	rendered := domain.CommandSpec{
		Exe:            render(spec.Exe, data),
		Args:           make([]string, 0, len(spec.Args)),
		Env:            make(map[string]string, len(spec.Env)),
		WorkingDir:     render(spec.WorkingDir, data),
		TimeoutSeconds: spec.TimeoutSeconds,
	}
	for index, arg := range spec.Args {
		if arg == "{{test.cargoTarget}}" {
			if data.Test == nil {
				return spec, fmt.Errorf("Cargo test target identity is missing; rediscover tests before selecting it")
			}
			args, err := CargoTargetArgs(data.Test.CargoPackage, data.Test.CargoTargetKind, data.Test.CargoTargetName)
			if err != nil {
				return spec, err
			}
			rendered.Args = append(rendered.Args, args...)
		} else if data.Provider.Kind == domain.ProviderDotNet && data.Test != nil && isDefaultDotNetFilter(spec, index) && (strings.Contains(data.Test.Selector, "(") || data.Test.DotNetFilterProperty == "FullyQualifiedName") {
			property := "DisplayName"
			if data.Test.DotNetFilterProperty == "FullyQualifiedName" {
				property = "FullyQualifiedName"
			}
			rendered.Args = append(rendered.Args, property+"="+escapeDotNetFilter(data.Test.Selector))
		} else {
			rendered.Args = append(rendered.Args, render(arg, data))
		}
	}
	if data.Provider.Kind == domain.ProviderGo && data.Test != nil && strings.HasPrefix(data.Test.Name, "Benchmark") && isBuiltInGoTarget(spec) {
		rendered.Args = []string{"test", data.Test.GoPackage, "-run", "^$", "-bench", "^" + regexp.QuoteMeta(data.Test.Selector) + "$", "-json"}
	}
	if data.Provider.Kind == domain.ProviderDotNet && isBuiltInDotNetRun(spec) {
		if data.Test != nil && data.Test.DotNetProject != "" {
			rendered.Args[1] = canonicalDotNetProject(data.Test.DotNetProject)
		}
		rendered.Args = append(rendered.Args, "--", "NUnit.DisplayName=FullName")
		if data.Test != nil && data.Test.DotNetNUnitLegacy {
			rendered.Args = append(rendered.Args, "NUnit.DiscoveryMethod=Legacy")
		}
	}
	for key, value := range spec.Env {
		rendered.Env[key] = render(value, data)
	}
	if rendered.WorkingDir == "" {
		rendered.WorkingDir = filepath.Join(data.ProjectRoot, data.Provider.Root)
	} else if !filepath.IsAbs(rendered.WorkingDir) {
		rendered.WorkingDir = filepath.Join(data.ProjectRoot, rendered.WorkingDir)
	}
	return rendered, nil
}

// canonicalDotNetProject resolves symbolic links in an absolute project path.
//
// Discovery builds from the working directory, which the operating system
// reports by its real path, while a stored project path may be spelled through
// a link (macOS temporary directories live under /var, a link to /private/var).
// Building the same project under two spellings makes NuGet and the xUnit and
// NUnit adapters lose the copied package assemblies, so every run must use the
// real path. If resolution fails the stored path is returned unchanged.
func canonicalDotNetProject(project string) string {
	if !filepath.IsAbs(project) {
		return project
	}
	if resolved, err := filepath.EvalSymlinks(project); err == nil {
		return resolved
	}
	return project
}

// CargoTargetArgs selects one package and one test target in Cargo.
func CargoTargetArgs(pkg, kind, name string) ([]string, error) {
	if pkg == "" || kind == "" || name == "" {
		return nil, fmt.Errorf("Cargo test target identity is missing; rediscover tests before selecting it")
	}
	switch kind {
	case "lib":
		return []string{"--package", pkg, "--lib"}, nil
	case "bin", "test", "example", "bench":
		return []string{"--package", pkg, "--" + kind, name}, nil
	default:
		return nil, fmt.Errorf("unsupported Cargo test target kind %q", kind)
	}
}

func isBuiltInGoTarget(spec domain.CommandSpec) bool {
	if strings.TrimSuffix(strings.ToLower(filepath.Base(spec.Exe)), ".exe") != "go" || len(spec.Args) != 5 {
		return false
	}
	return spec.Args[0] == "test" && spec.Args[1] == "{{test.goPackage}}" && spec.Args[2] == "-run" && spec.Args[3] == "^{{test.selector}}$" && spec.Args[4] == "-json"
}

func isBuiltInDotNetRun(spec domain.CommandSpec) bool {
	if strings.TrimSuffix(strings.ToLower(filepath.Base(spec.Exe)), ".exe") != "dotnet" {
		return false
	}
	args := spec.Args
	if len(args) != 6 && len(args) != 8 {
		return false
	}
	projectFile := filepath.Base(args[1]) == args[1] && strings.EqualFold(filepath.Ext(args[1]), ".csproj")
	if args[0] != "test" || (args[1] != "{{provider.root}}" && args[1] != "." && !projectFile) {
		return false
	}
	i := 2
	if len(args) == 8 {
		if args[2] != "--filter" || args[3] != "FullyQualifiedName={{test.selector}}" {
			return false
		}
		i = 4
	}
	return args[i] == "--logger" && args[i+1] == "trx" && args[i+2] == "--results-directory" && args[i+3] == "{{artifact.dir}}"
}

func isDefaultDotNetFilter(spec domain.CommandSpec, index int) bool {
	exe := strings.TrimSuffix(strings.ToLower(filepath.Base(spec.Exe)), ".exe")
	return exe == "dotnet" && index > 0 && spec.Args[index-1] == "--filter" && spec.Args[index] == "FullyQualifiedName={{test.selector}}"
}

// escapeDotNetFilter protects VSTest filter syntax and MSBuild argument parsing.
//
// A literal backslash needs two backslashes in the VSTest filter grammar. The
// dotnet CLI then re-parses a run of backslashes that precedes a quote with
// Windows command-line rules on every operating system, so a run of n
// backslashes directly before an encoded quote is written as 4n-1. The rule was
// verified on Windows and Linux against display names with quotes, trailing
// backslashes and runs of backslashes; it must not depend on runtime.GOOS.
func escapeDotNetFilter(value string) string {
	runes := []rune(value)
	var result strings.Builder
	for index := 0; index < len(runes); {
		char := runes[index]
		if char == '\\' {
			end := index
			for end < len(runes) && runes[end] == '\\' {
				end++
			}
			count := end - index
			width := 2 * count
			if end < len(runes) && runes[end] == '"' {
				width = 4*count - 1
			}
			result.WriteString(strings.Repeat(`\`, width))
			index = end
			continue
		}
		switch char {
		case '%':
			result.WriteString("%25")
		case ',':
			result.WriteString("%2C")
		case '"':
			result.WriteString("%22")
		case ';':
			result.WriteString("%3B")
		case '(', ')', '&', '|', '=', '!', '~':
			result.WriteByte('\\')
			result.WriteRune(char)
		default:
			result.WriteRune(char)
		}
		index++
	}
	return result.String()
}

func render(value string, data TemplateData) string {
	return placeholderPattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := placeholderPattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		key := strings.TrimSpace(parts[1])
		switch key {
		case "project.root":
			return data.ProjectRoot
		case "provider.id":
			return data.Provider.ID
		case "provider.name":
			return data.Provider.Name
		case "provider.root":
			return data.Provider.Root
		case "provider.kind":
			return string(data.Provider.Kind)
		case "artifact.path":
			return data.ArtifactPath
		case "artifact.name":
			return filepath.Base(data.ArtifactPath)
		case "artifact.dir":
			return data.ArtifactPath + ".d"
		case "test.id":
			if data.Test == nil {
				return ""
			}
			return data.Test.ID
		case "test.name":
			if data.Test == nil {
				return ""
			}
			return data.Test.Name
		case "test.fullName":
			if data.Test == nil {
				return ""
			}
			return data.Test.FullName
		case "test.selector":
			if data.Test == nil {
				return ""
			}
			return data.Test.Selector
		case "test.goPackage":
			if data.Test == nil {
				return ""
			}
			return data.Test.GoPackage
		case "test.file":
			if data.Test == nil {
				return ""
			}
			return data.Test.File
		default:
			return match
		}
	})
}

func Preview(spec domain.CommandSpec) string {
	parts := append([]string{spec.Exe}, spec.Args...)
	for idx, part := range parts {
		if strings.ContainsAny(part, " \t\"'") {
			parts[idx] = fmt.Sprintf("%q", part)
		}
	}
	return strings.Join(parts, " ")
}
