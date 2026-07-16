package java

import (
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gitbagHero/EnvMason/internal/inventory"
)

const maxReleaseFileSize = 128 * 1024

func parseJavaHomeXML(data string, home string) ([]JDKInstallation, error) {
	decoder := xml.NewDecoder(strings.NewReader(data))
	installations := []JDKInstallation{}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("invalid java_home XML")
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "dict" {
			continue
		}
		values, parseErr := parsePlistDict(decoder)
		if parseErr != nil {
			return nil, parseErr
		}
		jdkHome := values["JVMHomePath"]
		version := values["JVMVersion"]
		if jdkHome == "" || version == "" {
			continue
		}
		installations = append(installations, JDKInstallation{
			ID: jdkID(jdkHome), Version: version, Home: redactHome(jdkHome, home),
			Name: values["JVMName"], Vendor: values["JVMVendor"], Architecture: parseArchitecture(values["JVMArch"]),
			Manager: ManagerSystem, Registered: true, JenvAliases: []string{},
		})
	}
	return installations, nil
}

func parsePlistDict(decoder *xml.Decoder) (map[string]string, error) {
	values := make(map[string]string)
	key := ""
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, errors.New("invalid java_home plist dict")
		}
		switch value := token.(type) {
		case xml.EndElement:
			if value.Name.Local == "dict" {
				return values, nil
			}
		case xml.StartElement:
			switch value.Name.Local {
			case "key":
				if err := decoder.DecodeElement(&key, &value); err != nil {
					return nil, errors.New("invalid java_home plist key")
				}
			case "string":
				var text string
				if err := decoder.DecodeElement(&text, &value); err != nil {
					return nil, errors.New("invalid java_home plist value")
				}
				if key != "" {
					values[key] = strings.TrimSpace(text)
					key = ""
				}
			}
		}
	}
}

func parseReleaseFile(path string) (version, vendor string, architecture inventory.Architecture, err error) {
	file, err := os.Open(filepath.Join(path, "release"))
	if err != nil {
		return "", "", inventory.ArchitectureUnknown, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxReleaseFileSize {
		return "", "", inventory.ArchitectureUnknown, errors.New("invalid JDK release metadata")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxReleaseFileSize+1))
	if err != nil || len(data) > maxReleaseFileSize {
		return "", "", inventory.ArchitectureUnknown, errors.New("invalid JDK release metadata")
	}
	values := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		unquoted, quoteErr := strconv.Unquote(strings.TrimSpace(value))
		if quoteErr != nil {
			unquoted = strings.Trim(strings.TrimSpace(value), `"`)
		}
		values[strings.TrimSpace(key)] = unquoted
	}
	version = values["JAVA_VERSION"]
	vendor = values["IMPLEMENTOR"]
	architecture = parseArchitecture(values["OS_ARCH"])
	if version == "" {
		return "", "", inventory.ArchitectureUnknown, errors.New("JDK release metadata lacks version")
	}
	return version, vendor, architecture, nil
}

func parseJavaRuntime(output, home string) Runtime {
	result := Runtime{State: StateUnknown, Version: unknown, Home: unknown, Vendor: unknown, Architecture: inventory.ArchitectureUnknown}
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "java.home":
			result.Home = redactHome(strings.TrimSpace(value), home)
		case "java.version":
			result.Version = safeValue(value)
		case "java.vendor":
			result.Vendor = safeValue(value)
		case "os.arch":
			result.Architecture = parseArchitecture(strings.TrimSpace(value))
		}
	}
	if result.Version != unknown && result.Home != unknown {
		result.State = StateInstalled
	}
	return result
}

func parseMaven(output, home string) BuildTool {
	result := BuildTool{State: StateUnknown, Name: "maven", Version: unknown, Home: unknown, JavaVersion: unknown, JavaHome: unknown}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Apache Maven "):
			fields := strings.Fields(strings.TrimPrefix(line, "Apache Maven "))
			if len(fields) > 0 {
				result.Version = safeValue(fields[0])
			}
		case strings.HasPrefix(line, "Maven home: "):
			result.Home = redactHome(strings.TrimSpace(strings.TrimPrefix(line, "Maven home: ")), home)
		case strings.HasPrefix(line, "Java version: "):
			value := strings.TrimPrefix(line, "Java version: ")
			version, rest, _ := strings.Cut(value, ",")
			result.JavaVersion = safeValue(version)
			if marker := strings.Index(rest, "runtime:"); marker >= 0 {
				result.JavaHome = redactHome(strings.TrimSpace(rest[marker+len("runtime:"):]), home)
			}
		}
	}
	if result.Version != unknown {
		result.State = StateInstalled
	}
	return result
}

func parseArchitecture(value string) inventory.Architecture {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "arm64", "aarch64":
		return inventory.ArchitectureARM64
	case "x86_64", "amd64":
		return inventory.ArchitectureAMD64
	case "x86", "i386", "i686", "386":
		return inventory.Architecture386
	default:
		return inventory.ArchitectureUnknown
	}
}

func safeValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 256 || strings.ContainsAny(value, "\r\n\x00") {
		return unknown
	}
	return value
}
