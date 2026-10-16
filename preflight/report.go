package preflight

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const GenericProxyScope = "this pod and customer-provided config, using generic rules only; customer-specific correctness is not verified"

type Status string

const (
	StatusPass    Status = "PASS"
	StatusWarn    Status = "WARN"
	StatusFail    Status = "FAIL"
	StatusSkip    Status = "SKIP"
	StatusUnknown Status = "UNKNOWN"
	StatusInfo    Status = "INFO"
)

type Result struct {
	Status  Status   `json:"status"`
	Name    string   `json:"name"`
	Details []string `json:"details,omitempty"`
}

type Report struct {
	ConfigPath   string   `json:"configPath"`
	ConfigSHA256 string   `json:"configSHA256"`
	Version      string   `json:"version"`
	Pod          string   `json:"pod"`
	Scope        string   `json:"scope"`
	Results      []Result `json:"results"`
}

func (r Report) HasFailures() bool {
	for _, result := range r.Results {
		if result.Status == StatusFail {
			return true
		}
	}
	return false
}

func (r Report) HasUnknowns() bool {
	for _, result := range r.Results {
		if result.Status == StatusUnknown {
			return true
		}
	}
	return false
}

func (r Report) WriteText(w io.Writer) error {
	var output bytes.Buffer
	_, _ = fmt.Fprintln(&output, "s2s-proxy validate: proxy self-check")
	_, _ = fmt.Fprintf(&output, "config  %s  sha256 %s\n", r.ConfigPath, r.ConfigSHA256)
	_, _ = fmt.Fprintf(&output, "proxy   %s  pod %s\n\n", r.Version, r.Pod)

	counts := make(map[Status]int)
	for _, result := range r.Results {
		counts[result.Status]++
		_, _ = fmt.Fprintf(&output, "%-8s%s\n", result.Status, result.Name)
		for _, detail := range result.Details {
			_, _ = fmt.Fprintf(&output, "        %s\n", detail)
		}
	}

	_, _ = fmt.Fprintf(
		&output,
		"\n%d failed, %d warnings, %d passed, %d skipped, %d unknown\n",
		counts[StatusFail],
		counts[StatusWarn],
		counts[StatusPass],
		counts[StatusSkip],
		counts[StatusUnknown],
	)
	if r.Scope != "" {
		_, _ = fmt.Fprintln(&output, "scope: "+r.Scope)
	}
	_, err := w.Write(output.Bytes())
	return err
}

func (r Report) WriteJSON(w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(r)
}
