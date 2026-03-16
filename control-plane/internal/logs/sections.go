package logs

import "strings"

func AppendSection(sb *strings.Builder, title string, content string) {
	sb.WriteString("=== ")
	sb.WriteString(title)
	sb.WriteString(" ===\\n")
	if strings.TrimSpace(content) == "" {
		sb.WriteString("(no output)\\n\\n")
		return
	}
	sb.WriteString(content)
	if !strings.HasSuffix(content, "\\n") {
		sb.WriteString("\\n")
	}
	sb.WriteString("\\n")
}
