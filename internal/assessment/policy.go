package assessment

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gitbagHero/EnvMason/internal/version"
)

const MaxPolicyBytes = 64 * 1024

func LoadPolicy(path string) (Policy, error) {
	if strings.TrimSpace(path) == "" {
		return DefaultPolicy(), nil
	}
	file, err := os.Open(path)
	if err != nil {
		return Policy{}, fmt.Errorf("open policy: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, MaxPolicyBytes+1))
	if err != nil {
		return Policy{}, fmt.Errorf("read policy: %w", err)
	}
	if len(data) > MaxPolicyBytes {
		return Policy{}, fmt.Errorf("read policy: file exceeds %d bytes", MaxPolicyBytes)
	}
	return DecodePolicy(data)
}

func DecodePolicy(data []byte) (Policy, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var policy Policy
	if err := decoder.Decode(&policy); err != nil {
		return Policy{}, fmt.Errorf("decode policy: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Policy{}, err
	}
	return ResolvePolicy(policy)
}

// ResolvePolicy validates a policy and fills deterministic defaults before it
// is evaluated or included in a Plan digest.
func ResolvePolicy(policy Policy) (Policy, error) {
	if err := ValidatePolicy(policy); err != nil {
		return Policy{}, err
	}
	return withDefaults(policy), nil
}

func ValidatePolicy(policy Policy) error {
	if policy.SchemaVersion != PolicySchemaVersion {
		return fmt.Errorf("validate policy: unsupported schema_version %q", policy.SchemaVersion)
	}
	if policy.Tools == nil {
		return errors.New("validate policy: tools is required")
	}
	for toolID, rule := range policy.Tools {
		if toolID != "runtime.node" && toolID != "runtime.java" {
			return fmt.Errorf("validate policy: unsupported tool %q", toolID)
		}
		if rule.Channel != "" && rule.Channel != ChannelLTS && rule.Channel != ChannelStable {
			return fmt.Errorf("validate policy: unsupported channel %q for %s", rule.Channel, toolID)
		}
		if rule.Pin == "" {
			continue
		}
		valid := version.ParseSemVer(rule.Pin).Comparable
		if toolID == "runtime.java" {
			valid = version.ParseJava(rule.Pin).Comparable
		}
		if !valid {
			return fmt.Errorf("validate policy: invalid pin %q for %s", rule.Pin, toolID)
		}
	}
	return nil
}

func withDefaults(policy Policy) Policy {
	defaults := DefaultPolicy()
	for toolID, rule := range policy.Tools {
		if rule.Channel == "" {
			rule.Channel = ChannelLTS
		}
		defaults.Tools[toolID] = rule
	}
	return defaults
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode policy: %w", err)
	}
	return errors.New("decode policy: trailing JSON value")
}
