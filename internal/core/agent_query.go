package core

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var agentContainerField = regexp.MustCompile(`\{\{\.(ID|Image|Command|CreatedAt|RunningFor|Ports|State|Status|Size|Names|Labels|Mounts|Networks)\}\}`)

// This recognizes a literal query subset; execution always uses the original
// command, never reconstructed words. Shell expansion remains approval-only.
func agentQueryAllowed(command string) bool {
	if command != "free" && !strings.HasPrefix(command, "free ") && command != "docker ps" && !strings.HasPrefix(command, "docker ps ") {
		return false
	}
	wordByte := func(b byte) bool {
		return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || strings.ContainsRune("_./-:", rune(b))
	}
	var words []string
	for i := 0; i < len(command); {
		if command[i] == ' ' || command[i] == '\t' {
			i++
			continue
		}
		var word strings.Builder
		if command[i] == '\'' || command[i] == '"' {
			quote := command[i]
			i++
			for i < len(command) && command[i] != quote {
				b := command[i]
				if b < 32 || b > 126 || b == '$' || b == '`' || quote == '"' && b == '\\' {
					return false
				}
				word.WriteByte(b)
				i++
			}
			if i == len(command) {
				return false
			}
			i++
			if i < len(command) && command[i] != ' ' && command[i] != '\t' {
				return false
			}
		} else {
			for i < len(command) && command[i] != ' ' && command[i] != '\t' {
				if !wordByte(command[i]) && command[i] != '=' {
					return false
				}
				word.WriteByte(command[i])
				i++
			}
		}
		words = append(words, word.String())
	}
	if words[0] == "free" {
		for _, arg := range words[1:] {
			if !slices.Contains([]string{"-b", "-k", "-m", "-g", "-h", "-w", "-t", "--bytes", "--kibi", "--mebi", "--gibi", "--human", "--wide", "--total", "--si"}, arg) {
				return false
			}
		}
		return true
	}
	if len(words) < 2 || words[0] != "docker" || words[1] != "ps" {
		return false
	}
	for i := 2; i < len(words); i++ {
		arg := words[i]
		if slices.Contains([]string{"-a", "--all", "-q", "--quiet", "--no-trunc"}, arg) {
			continue
		}
		flag, value, found := strings.Cut(arg, "=")
		if !found {
			i++
			if i == len(words) {
				return false
			}
			value = words[i]
		}
		switch flag {
		case "--format":
			if value == "json" {
				continue
			}
			format := strings.TrimPrefix(value, "table ")
			if !agentContainerField.MatchString(format) {
				return false
			}
			separators := strings.NewReplacer(`\t`, "", `\n`, "").Replace(agentContainerField.ReplaceAllString(format, ""))
			if strings.Trim(separators, " |") != "" {
				return false
			}
		case "-n", "--last":
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 || n > 1000 {
				return false
			}
		case "-f", "--filter":
			key, filter, ok := strings.Cut(value, "=")
			if !ok || filter == "" || !slices.Contains([]string{"id", "name", "status"}, key) {
				return false
			}
			for i := range len(filter) {
				if !wordByte(filter[i]) {
					return false
				}
			}
		default:
			return false
		}
	}
	return true
}
