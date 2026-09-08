// ABOUTME: Validates the public invocation without allocating preview output.
// ABOUTME: All resource limits have explicit finite defaults and ranges.
package preview

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type config struct {
	links                                bool
	mode, root                           string
	grace, deadline                      time.Duration
	files, depth                         int64
	sourceBytes, totalBytes, outputBytes int64
}

func arguments(args []string) ([]string, string, error) {
	var files []string
	info := ""
	options := true
	for _, arg := range args {
		if options && arg == "--" {
			options = false
			continue
		}
		if options && strings.HasPrefix(arg, "-") {
			if (arg == "-h" || arg == "--help" || arg == "--version") && info == "" {
				info = arg
			} else {
				return nil, "", fmt.Errorf("unknown or combined option %q", arg)
			}
		} else {
			files = append(files, arg)
		}
	}
	if info != "" && len(args) != 1 {
		return nil, "", fmt.Errorf("informational options must be used alone")
	}
	if info == "" && len(files) == 0 {
		return nil, "", fmt.Errorf("provide at least one Markdown or Org file; see --help")
	}
	return files, info, nil
}

func settings(env []string) (config, error) {
	c := config{mode: "quick", grace: 3 * time.Second, deadline: time.Minute, files: 50, depth: 3, sourceBytes: 10485760, totalBytes: 52428800, outputBytes: 104857600}
	values := make(map[string]string)
	for _, entry := range env {
		key, value, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "HTMLPREVIEW_") {
			values[key] = value
		}
	}
	for key, value := range values {
		switch key {
		case "HTMLPREVIEW_LINKS":
			if value != "" && value != "0" && value != "1" {
				return c, fmt.Errorf("%s must be 0 or 1", key)
			}
			c.links = value == "1"
		case "HTMLPREVIEW_MODE":
			if value != "" && value != "quick" && value != "read" {
				return c, fmt.Errorf("%s must be quick or read", key)
			}
		case "HTMLPREVIEW_ROOT":
			c.root = value
		case "HTMLPREVIEW_GRACE", "HTMLPREVIEW_DEADLINE":
			if value == "" {
				continue
			}
			d, err := time.ParseDuration(value)
			max := time.Hour
			if key == "HTMLPREVIEW_DEADLINE" {
				max = 10 * time.Minute
			}
			if err != nil || d < 100*time.Millisecond || d > max {
				return c, fmt.Errorf("%s requires a duration from 100ms to %s", key, max)
			}
			if key == "HTMLPREVIEW_GRACE" {
				c.grace = d
			} else {
				c.deadline = d
			}
		case "HTMLPREVIEW_MAX_FILES", "HTMLPREVIEW_MAX_DEPTH", "HTMLPREVIEW_MAX_SOURCE_BYTES", "HTMLPREVIEW_MAX_TOTAL_SOURCE_BYTES", "HTMLPREVIEW_MAX_OUTPUT_BYTES":
			if value == "" {
				continue
			}
			min, max := int64(1), int64(500)
			switch key {
			case "HTMLPREVIEW_MAX_DEPTH":
				min, max = 0, 10
			case "HTMLPREVIEW_MAX_SOURCE_BYTES":
				max = 10485760
			case "HTMLPREVIEW_MAX_TOTAL_SOURCE_BYTES":
				max = 52428800
			case "HTMLPREVIEW_MAX_OUTPUT_BYTES":
				max = 104857600
			}
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || n < min || n > max {
				return c, fmt.Errorf("%s must be an integer from %d to %d", key, min, max)
			}
			switch key {
			case "HTMLPREVIEW_MAX_FILES":
				c.files = n
			case "HTMLPREVIEW_MAX_DEPTH":
				c.depth = n
			case "HTMLPREVIEW_MAX_SOURCE_BYTES":
				c.sourceBytes = n
			case "HTMLPREVIEW_MAX_TOTAL_SOURCE_BYTES":
				c.totalBytes = n
			case "HTMLPREVIEW_MAX_OUTPUT_BYTES":
				c.outputBytes = n
			}
		default:
			return c, fmt.Errorf("unknown setting %s", key)
		}
	}
	if c.links {
		c.mode = "read"
	}
	if value := values["HTMLPREVIEW_MODE"]; value != "" {
		c.mode = value
	}
	if c.links && c.mode == "quick" {
		return c, fmt.Errorf("HTMLPREVIEW_LINKS=1 requires HTMLPREVIEW_MODE=read")
	}
	return c, nil
}
