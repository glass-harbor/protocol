//go:build regression

package regression

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"gopkg.in/yaml.v3"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
)

// op is one YAML document. One struct for all types keeps parsing trivial; Type selects the fields used.
type op struct {
	Type string `yaml:"type"`
	// state
	Genesis map[string]any `yaml:"genesis"`
	// tx
	Signer   string           `yaml:"signer"`
	Msgs     []map[string]any `yaml:"msgs"`
	Gas      uint64           `yaml:"gas"`
	Sequence *uint64          `yaml:"sequence"`
	Error    string           `yaml:"error"`
	// create-blocks
	Count int `yaml:"count"`
	// check
	Endpoint string            `yaml:"endpoint"`
	Params   map[string]string `yaml:"params"`
	Asserts  []string          `yaml:"asserts"`

	line int
}

var tmplFuncs = template.FuncMap{
	"addr":    func(name string) string { return mustAddr(name).String() },
	"valoper": func(name string) string { return sdk.ValAddress(mustAddr(name)).String() },
	"module":  func(name string) string { return authtypes.NewModuleAddress(name).String() },
}

func mustAddr(name string) sdk.AccAddress {
	a, ok := addrs[name]
	if !ok {
		panic(fmt.Sprintf("unknown key %q (known: validator, treasury, alice, bob, carol)", name))
	}
	return a
}

// render executes the file as a Go template.
func render(t *testing.T, path string) []byte {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := template.New(filepath.Base(path)).Funcs(tmplFuncs).Parse(string(src))
	if err != nil {
		t.Fatalf("%s: template: %v", path, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, nil); err != nil {
		t.Fatalf("%s: template: %v", path, err)
	}
	return buf.Bytes()
}

// parseOps renders and splits a suite into operations; state ops must come first.
func parseOps(t *testing.T, path string) []op {
	t.Helper()
	dec := yaml.NewDecoder(bytes.NewReader(render(t, path)))
	var ops []op
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("%s: yaml: %v", path, err)
		}
		if doc.Kind == 0 { // empty document (e.g. trailing ---)
			continue
		}
		var o op
		if err := doc.Decode(&o); err != nil {
			t.Fatalf("%s:%d: yaml: %v", path, doc.Line, err)
		}
		o.line = doc.Line
		// yaml.v3 ignores unknown keys, so a misspelled field would silently produce a no-op.
		switch o.Type {
		case "state":
			if o.Genesis == nil {
				t.Fatalf("%s:%d: state op needs `genesis`", path, o.line)
			}
		case "tx":
			if o.Signer == "" || len(o.Msgs) == 0 {
				t.Fatalf("%s:%d: tx op needs `signer` and `msgs`", path, o.line)
			}
		case "create-blocks":
		case "check":
			if o.Endpoint == "" || len(o.Asserts) == 0 {
				t.Fatalf("%s:%d: check op needs `endpoint` and `asserts`", path, o.line)
			}
		default:
			t.Fatalf("%s:%d: unknown op type %q", path, o.line, o.Type)
		}
		ops = append(ops, o)
	}
	seenOther := false
	for _, o := range ops {
		if o.Type == "state" && seenOther {
			t.Fatalf("%s:%d: state ops must precede all other ops", path, o.line)
		}
		if o.Type != "state" {
			seenOther = true
		}
	}
	if len(ops) == 0 {
		t.Fatalf("%s: no operations", path)
	}
	return ops
}

// yamlToJSON converts a decoded YAML map to JSON bytes for jq.
func yamlToJSON(t *testing.T, v any) []byte {
	t.Helper()
	bz, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return bz
}

// runSuite executes one YAML file against a fresh node.
func runSuite(t *testing.T, path string) {
	t.Helper()
	ops := parseOps(t, path)

	var defaultState map[string]any
	if err := yaml.Unmarshal(render(t, "templates/default-state.yaml"), &defaultState); err != nil {
		t.Fatalf("default-state.yaml: %v", err)
	}
	overlays := [][]byte{yamlToJSON(t, defaultState)}
	rest := ops
	for len(rest) > 0 && rest[0].Type == "state" {
		overlays = append(overlays, yamlToJSON(t, rest[0].Genesis))
		rest = rest[1:]
	}

	n := startNode(t, path, overlays)
	for _, o := range rest {
		switch o.Type {
		case "tx":
			n.tx(o)
		case "create-blocks":
			n.createBlocks(o)
		case "check":
			n.check(o)
		}
	}
	if len(n.pending) > 0 {
		n.fatalf(n.pending[0].o, "%d tx(s) never confirmed: add a trailing create-blocks", len(n.pending))
	}
}

// createBlocks releases count blocks and verifies pending txs landed in the first one.
func (n *node) createBlocks(o op) {
	count := o.Count
	if count == 0 {
		count = 1
	}
	for i := 0; i < count; i++ {
		h := n.newBlock()
		if i == 0 {
			n.verifyPending(o, h)
		}
	}
}

// check fetches an endpoint and runs each assert through `jq -e`.
func (n *node) check(o op) {
	// A check before the block that applies pending txs is almost always a suite bug: it
	// observes state as of the last committed block, not the txs the suite just submitted.
	if len(n.pending) > 0 {
		n.fatalf(o, "check while %d tx(s) are pending: add a create-blocks first", len(n.pending))
	}
	u := o.Endpoint
	if !strings.HasPrefix(u, "http") {
		u = n.api + u
	}
	if len(o.Params) > 0 {
		q := url.Values{}
		for k, v := range o.Params {
			q.Set(k, v)
		}
		u += "?" + q.Encode()
	}
	code, body, err := n.httpGet(u)
	if err != nil {
		n.fatalf(o, "GET %s failed: %v", u, err)
	}
	for _, a := range o.Asserts {
		cmd := exec.Command("jq", "-e", a)
		cmd.Stdin = bytes.NewReader(body)
		out, err := cmd.CombinedOutput()
		if err != nil {
			var pretty bytes.Buffer
			if json.Indent(&pretty, body, "", "  ") != nil {
				pretty.Write(body)
			}
			n.fatalf(o, "assert failed: %s\njq: %s\nGET %s -> %d\n%s", a, strings.TrimSpace(string(out)), u, code, pretty.String())
		}
	}
}
