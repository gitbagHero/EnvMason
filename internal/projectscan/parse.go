package projectscan

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"sort"
	"strings"

	versioncore "github.com/gitbagHero/EnvMason/internal/version"
)

var (
	nodeExactPattern     = regexp.MustCompile(`^v?[0-9]+(?:\.[0-9]+){0,2}$`)
	nodeAliasPattern     = regexp.MustCompile(`^(?:node|stable|lts(?:/[A-Za-z0-9._-]+)?)$`)
	simpleRangeToken     = regexp.MustCompile(`^(>=|<=|>|<|=)?(v?[0-9]+(?:\.[0-9]+){0,2})$`)
	gradleJavaVersion    = regexp.MustCompile(`(?m)(?:sourceCompatibility|targetCompatibility)\s*=\s*(?:JavaVersion\.)?VERSION_((?:1_)?[0-9]+)`)
	gradleQuotedVersion  = regexp.MustCompile(`(?m)(?:sourceCompatibility|targetCompatibility)\s*=\s*['"]([0-9]+(?:\.[0-9]+){0,2})['"]`)
	gradleNumericVersion = regexp.MustCompile(`(?m)(?:sourceCompatibility|targetCompatibility)\s*=\s*([0-9]+(?:\.[0-9]+){0,2})(?:\s|$)`)
	gradleToolchain      = regexp.MustCompile(`JavaLanguageVersion\.of\(\s*([0-9]+)\s*\)`)
	gradleDynamic        = regexp.MustCompile(`(?m)(?:sourceCompatibility|targetCompatibility|languageVersion)\s*(?:=|\.set\().*[$A-Za-z_]`)
)

func parseFile(name string, body []byte) ([]Reference, error) {
	switch name {
	case ".nvmrc", ".node-version":
		value, ok := firstValue(body)
		if !ok {
			return nil, errors.New("empty Node version file")
		}
		return []Reference{nodeReference(value, name)}, nil
	case ".java-version":
		value, ok := firstValue(body)
		if !ok {
			return nil, errors.New("empty Java version file")
		}
		return []Reference{javaReference(value, name)}, nil
	case "package.json":
		return parsePackageJSON(body)
	case ".tool-versions":
		return parseToolVersions(body)
	case "pom.xml":
		return parsePOM(body)
	case "build.gradle", "build.gradle.kts":
		return parseGradle(body, name), nil
	default:
		return nil, nil
	}
}

func firstValue(body []byte) (string, bool) {
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line != "" && !strings.HasPrefix(line, "#") {
			return line, true
		}
	}
	return "", false
}

func parsePackageJSON(body []byte) ([]Reference, error) {
	var document struct {
		Engines map[string]json.RawMessage `json:"engines"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, errors.New("trailing package JSON")
	}
	raw, exists := document.Engines["node"]
	if !exists {
		return nil, nil
	}
	var value string
	if json.Unmarshal(raw, &value) != nil || strings.TrimSpace(value) == "" {
		return nil, errors.New("invalid package engines.node")
	}
	return []Reference{nodeReference(value, "package.json")}, nil
}

func parseToolVersions(body []byte) ([]Reference, error) {
	references := []Reference{}
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if comment := strings.IndexByte(line, '#'); comment >= 0 {
			line = strings.TrimSpace(line[:comment])
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, errors.New("invalid tool-versions line")
		}
		switch fields[0] {
		case "nodejs", "node":
			for _, value := range fields[1:] {
				references = append(references, nodeReference(value, ".tool-versions"))
			}
		case "java":
			for _, value := range fields[1:] {
				references = append(references, javaReference(value, ".tool-versions"))
			}
		}
	}
	return references, nil
}

func parsePOM(body []byte) ([]Reference, error) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	accepted := map[string]bool{"java.version": true, "maven.compiler.release": true, "maven.compiler.source": true, "maven.compiler.target": true}
	references := []Reference{}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		start, ok := token.(xml.StartElement)
		if !ok || !accepted[start.Name.Local] {
			continue
		}
		var value string
		if err := decoder.DecodeElement(&value, &start); err != nil {
			return nil, err
		}
		value = strings.TrimSpace(value)
		if value != "" {
			references = append(references, javaReference(value, "pom.xml"))
		}
	}
	return references, nil
}

func parseGradle(body []byte, name string) []Reference {
	values := map[string]bool{}
	for _, pattern := range []*regexp.Regexp{gradleJavaVersion, gradleQuotedVersion, gradleNumericVersion, gradleToolchain} {
		for _, match := range pattern.FindAllSubmatch(body, -1) {
			values[strings.ReplaceAll(string(match[1]), "_", ".")] = true
		}
	}
	references := make([]Reference, 0, len(values)+1)
	for value := range values {
		references = append(references, javaReference(value, name))
	}
	if len(references) == 0 && gradleDynamic.Match(body) {
		references = append(references, Reference{Runtime: RuntimeJava, Constraint: "dynamic", Kind: ConstraintUnknown, File: name})
	}
	sort.Slice(references, func(i, j int) bool { return references[i].Constraint < references[j].Constraint })
	return references
}

func nodeReference(raw, file string) Reference {
	raw = strings.TrimSpace(raw)
	reference := Reference{Runtime: RuntimeNode, Constraint: raw, Kind: ConstraintUnknown, File: file}
	switch {
	case nodeExactPattern.MatchString(raw):
		normalized := normalizeNodeExact(raw)
		if versioncore.ParseSemVer(normalized).Comparable {
			reference.Normalized, reference.Kind = normalized, ConstraintExact
		}
	case nodeAliasPattern.MatchString(strings.ToLower(raw)):
		reference.Kind = ConstraintAlias
	case parseSimpleRange(raw) != nil:
		reference.Kind = ConstraintRange
	}
	if reference.Kind == ConstraintUnknown {
		reference.Constraint = "unknown"
	}
	return reference
}

func javaReference(raw, file string) Reference {
	raw = strings.TrimSpace(raw)
	reference := Reference{Runtime: RuntimeJava, Constraint: raw, Kind: ConstraintUnknown, File: file}
	parsed := versioncore.ParseJava(raw)
	if parsed.Comparable {
		reference.Normalized, reference.Kind = parsed.Normalized, ConstraintExact
	}
	if reference.Kind == ConstraintUnknown {
		reference.Constraint = "unknown"
	}
	return reference
}

func normalizeNodeExact(raw string) string {
	raw = strings.TrimPrefix(raw, "v")
	parts := strings.Split(raw, ".")
	for len(parts) < 3 {
		parts = append(parts, "0")
	}
	return strings.Join(parts, ".")
}

type rangeBound struct {
	operator string
	version  string
}

func parseSimpleRange(raw string) []rangeBound {
	if strings.Contains(raw, "||") || strings.ContainsAny(raw, "^~*xX") {
		return nil
	}
	fields := strings.Fields(strings.ReplaceAll(raw, ",", " "))
	if len(fields) == 0 {
		return nil
	}
	result := make([]rangeBound, 0, len(fields))
	for _, field := range fields {
		match := simpleRangeToken.FindStringSubmatch(field)
		if match == nil {
			return nil
		}
		operator := match[1]
		if operator == "" && len(fields) > 1 {
			return nil
		}
		result = append(result, rangeBound{operator: operator, version: normalizeNodeExact(match[2])})
	}
	return result
}

func conflictIssues(project Project) []Issue {
	issues := []Issue{}
	for _, runtime := range []Runtime{RuntimeNode, RuntimeJava} {
		references := referencesFor(project.References, runtime)
		exact := []string{}
		for _, reference := range references {
			if reference.Kind == ConstraintExact && !containsEquivalent(exact, reference.Normalized, runtime) {
				exact = append(exact, reference.Normalized)
			}
		}
		conflict := len(exact) > 1
		if runtime == RuntimeNode && len(exact) == 1 {
			for _, reference := range references {
				if reference.Kind == ConstraintRange && !satisfiesRange(exact[0], parseSimpleRange(reference.Constraint)) {
					conflict = true
				}
			}
		}
		if conflict {
			issues = append(issues, Issue{Code: "PROJECT_VERSION_CONFLICT", Root: project.Root, Runtime: runtime, Details: conflictDetails(references)})
		}
	}
	return issues
}

func containsEquivalent(values []string, candidate string, runtime Runtime) bool {
	for _, value := range values {
		var relation versioncore.Relation
		if runtime == RuntimeNode {
			relation = versioncore.Compare(versioncore.ParseSemVer(value), versioncore.ParseSemVer(candidate))
		} else {
			relation = versioncore.Compare(versioncore.ParseJava(value), versioncore.ParseJava(candidate))
		}
		if relation == versioncore.RelationEqual {
			return true
		}
	}
	return false
}

func referencesFor(values []Reference, runtime Runtime) []Reference {
	result := []Reference{}
	for _, value := range values {
		if value.Runtime == runtime {
			result = append(result, value)
		}
	}
	return result
}

func conflictDetails(values []Reference) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.File+"="+value.Constraint)
	}
	sort.Strings(result)
	return result
}

func satisfiesRange(exact string, bounds []rangeBound) bool {
	if len(bounds) == 0 {
		return true
	}
	value := versioncore.ParseSemVer(exact)
	for _, bound := range bounds {
		relation := versioncore.Compare(value, versioncore.ParseSemVer(bound.version))
		switch bound.operator {
		case ">":
			if relation != versioncore.RelationGreater {
				return false
			}
		case ">=":
			if relation == versioncore.RelationLess {
				return false
			}
		case "<":
			if relation != versioncore.RelationLess {
				return false
			}
		case "<=":
			if relation == versioncore.RelationGreater {
				return false
			}
		case "", "=":
			if relation != versioncore.RelationEqual {
				return false
			}
		}
	}
	return true
}
