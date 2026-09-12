// ABOUTME: Checks staged service artefacts without activating a service manager.
// ABOUTME: Parses generated configuration and launchd structure at the install boundary.
package integration

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type plistElement struct {
	XMLName  xml.Name
	Text     string         `xml:",chardata"`
	Children []plistElement `xml:",any"`
}

func plistDictionary(t *testing.T, node plistElement) map[string]plistElement {
	t.Helper()
	if node.XMLName.Local != "dict" || len(node.Children)%2 != 0 {
		t.Fatal("invalid plist dictionary")
	}
	values := make(map[string]plistElement)
	for i := 0; i < len(node.Children); i += 2 {
		if node.Children[i].XMLName.Local != "key" {
			t.Fatal("plist dictionary key required")
		}
		values[node.Children[i].Text] = node.Children[i+1]
	}
	return values
}

func TestRT006_10_StagedServiceArtefacts(t *testing.T) {
	for _, prefix := range []string{"", "/test-prefix space &% $literal"} {
		t.Run(prefix, func(t *testing.T) {
			stage := t.TempDir()
			if out, err := stagedInstall(t, stage, "PREFIX="+prefix); err != nil {
				t.Fatalf("staged install: %v %s", err, out)
			}
			home, err := os.UserHomeDir()
			if err != nil {
				t.Fatal(err)
			}
			root, executable := filepath.Join(stage, prefix, "share/htmlpreview"), filepath.Join(prefix, "bin/htmlpreview")
			if prefix == "" {
				root = filepath.Join(stage, home, ".local/share/htmlpreview")
				executable, err = filepath.EvalSymlinks("../bin/htmlpreview")
				if err != nil {
					t.Fatal(err)
				}
				executable, err = filepath.Abs(executable)
				if err != nil {
					t.Fatal(err)
				}
			}
			// #nosec G304 -- Reads the package staged by this test under its temporary destination.
			data, err := os.ReadFile(filepath.Join(root, "config.example.yaml"))
			if err != nil {
				entries, listErr := os.ReadDir(stage)
				var names []string
				for _, entry := range entries {
					names = append(names, entry.Name())
				}
				t.Fatalf("installation lacks its empty-root config example: %v; staged roots=%v read=%v", err, names, listErr)
			}
			var cfg struct {
				Version int `yaml:"version"`
				Serve   struct {
					Roots []string `yaml:"roots"`
				} `yaml:"serve"`
			}
			if yaml.Unmarshal(data, &cfg) != nil || cfg.Version != 1 || cfg.Serve.Roots == nil || len(cfg.Serve.Roots) != 0 {
				t.Fatal("example grants roots or lacks versioned empty list")
			}
			if info, err := os.Stat(filepath.Join(root, "SERVICE.md")); err != nil || info.Size() == 0 {
				t.Fatal("service guide not packaged")
			}
			if runtime.GOOS == "darwin" {
				checkInstalledLaunchAgent(t, root, stage, executable)
			} else {
				// #nosec G304 -- The unit path is inside this test's staged package.
				data, err := os.ReadFile(filepath.Join(root, "service/htmlpreview.service"))
				if err != nil || strings.Contains(string(data), stage) || strings.Contains(string(data), "{{") {
					t.Fatal("installed unit missing or retains substitutions/staging path")
				}
			}
			for _, active := range []string{filepath.Join(home, "Library/LaunchAgents/org.htmlpreview.agent.plist"), filepath.Join(home, ".config/systemd/user/htmlpreview.service")} {
				if _, err := os.Lstat(filepath.Join(stage, active)); !os.IsNotExist(err) {
					t.Fatal("installer created an active service-manager artefact")
				}
			}
		})
	}
}

func checkInstalledLaunchAgent(t *testing.T, root, stage, executable string) {
	t.Helper()
	// #nosec G304 -- The caller supplies this test's staged shared-data directory.
	data, err := os.ReadFile(filepath.Join(root, "service/org.htmlpreview.agent.plist"))
	if err != nil {
		t.Fatal("installed LaunchAgent missing")
	}
	var plist plistElement
	if xml.Unmarshal(data, &plist) != nil || len(plist.Children) != 1 {
		t.Fatal("generated LaunchAgent is not a plist")
	}
	values := plistDictionary(t, plist.Children[0])
	args := values["ProgramArguments"].Children
	if len(args) != 2 || args[0].Text != executable || args[1].Text != "--serve" {
		t.Fatal("service executable arguments changed or contain staging path")
	}
	if values["Label"].Text != "org.htmlpreview.agent" || values["RunAtLoad"].XMLName.Local != "true" || values["ThrottleInterval"].Text != "5" || plistDictionary(t, values["KeepAlive"])["SuccessfulExit"].XMLName.Local != "false" {
		t.Fatal("launchd lifecycle policy differs")
	}
	env := plistDictionary(t, values["EnvironmentVariables"])
	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if env["HTMLPREVIEW_CONFIG"].Text != filepath.Join(base, "htmlpreview/config.yaml") || !strings.Contains(env["PATH"].Text, "/usr/bin") || strings.Contains(string(data), stage) {
		t.Fatal("service environment contains wrong configuration or staging paths")
	}
}
