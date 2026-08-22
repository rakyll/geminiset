package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rakyll/geminiset/pkg/gemini"
)

const (
	Reset   = "\033[0m"
	Bold    = "\033[1m"
	Dim     = "\033[2m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"
)

// PrintBanner prints the GeminiSet banner.
func PrintBanner() {
	banner := `
   ____                _       _ ____       _   
  / ___| ___ _ __ ___ (_)_ __ (_) ___|  ___| |_ 
 | |  _ / _ \ '_ ' _ \| | '_ \| \___ \ / _ \ __|
 | |_| |  __/ | | | | | | | | | |___) |  __/ |_ 
  \____|\___|_| |_| |_|_|_| |_|_|____/ \___|\__|
  Gemini-Native Kubernetes Workloads & Scheduling
`
	fmt.Printf("%s%s%s\n", Cyan, banner, Reset)
}

// PrintRationaleCard renders an AI decision summary in terminal with compact spacing, aligned score bars, and styled alternatives.
func PrintRationaleCard(podName, nodeName string, scores map[string]int, rationale string, alternatives ...gemini.AlternativeEvaluation) {
	fmt.Printf("%s%sGemini Scheduling Decision%s\n", Bold, Cyan, Reset)
	fmt.Printf("%s%s%s\n", Dim, strings.Repeat("─", 60), Reset)
	fmt.Printf("Pod:           %s%s%s\n", White, podName, Reset)
	fmt.Printf("Assigned Node: %s%s%s\n", Green, nodeName, Reset)
	if len(scores) > 0 {
		fmt.Printf("%sScores:%s\n", Bold, Reset)
		var keys []string
		maxKeyLen := 0
		for k := range scores {
			keys = append(keys, k)
			if len(k) > maxKeyLen {
				maxKeyLen = len(k)
			}
		}
		sort.Strings(keys)

		for _, k := range keys {
			v := scores[k]
			bar := renderProgressBar(v, 15)
			fmt.Printf("  • %-*s [%s] %3d/100\n", maxKeyLen+1, k+":", bar, v)
		}
	}
	fmt.Printf("%sRationale:%s\n", Bold, Reset)
	wrapped := wrapText(rationale, 80)
	for _, line := range wrapped {
		fmt.Printf("  %s\n", line)
	}
	if len(alternatives) > 0 {
		fmt.Printf("%sAlternatives Evaluated:%s\n", Bold, Reset)
		for _, a := range alternatives {
			altHeader := fmt.Sprintf("Node %s (Score: %d/100):", a.NodeName, a.Score)
			fmt.Printf("  • %s%s%s\n", Bold, altHeader, Reset)
			reasonWrapped := wrapText(a.Reason, 76)
			for _, line := range reasonWrapped {
				fmt.Printf("    %s\n", line)
			}
		}
	}
}

func renderProgressBar(score int, width int) string {
	filled := (score * width) / 100
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	if score >= 85 {
		return Green + bar + Reset
	} else if score >= 60 {
		return Yellow + bar + Reset
	}
	return Red + bar + Reset
}

func wrapText(text string, maxLen int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	var currentLine string
	for _, w := range words {
		if len(currentLine)+len(w)+1 > maxLen {
			lines = append(lines, currentLine)
			currentLine = w
		} else {
			if currentLine == "" {
				currentLine = w
			} else {
				currentLine += " " + w
			}
		}
	}
	if currentLine != "" {
		lines = append(lines, currentLine)
	}
	return lines
}
