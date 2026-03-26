package workflow

import (
	"fmt"
	"sort"
	"strings"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

// Diff compares two workflow definitions and returns a human-readable diff string.
// If old is nil, all fields in new are shown as additions (new workflow).
func Diff(old, new *Workflow) string {
	var b strings.Builder

	if old == nil {
		fmt.Fprintf(&b, "%s%s+ new workflow: %s%s\n", colorBold, colorGreen, new.Name, colorReset)
		if new.Description != "" {
			fmt.Fprintf(&b, "  %s+ description: %s%s\n", colorGreen, new.Description, colorReset)
		}
		for name, inp := range new.Inputs {
			fmt.Fprintf(&b, "  %s+ input %q: type=%s%s\n", colorGreen, name, inp.Type, colorReset)
		}
		for _, step := range new.Steps {
			fmt.Fprintf(&b, "  %s+ step %q: action=%s%s\n", colorGreen, step.Name, step.Action, colorReset)
		}
		return b.String()
	}

	// Description.
	if old.Description != new.Description {
		fmt.Fprintf(&b, "  %s~ description: %q → %q%s\n", colorYellow, old.Description, new.Description, colorReset)
	}

	// Inputs.
	diffInputs(&b, old.Inputs, new.Inputs)

	// Steps.
	diffSteps(&b, old.Steps, new.Steps)

	if b.Len() == 0 {
		return ""
	}

	header := fmt.Sprintf("%s%s~ workflow: %s%s\n", colorBold, colorCyan, new.Name, colorReset)
	return header + b.String()
}

func diffInputs(b *strings.Builder, old, new map[string]Input) {
	allNames := make(map[string]bool)
	for name := range old {
		allNames[name] = true
	}
	for name := range new {
		allNames[name] = true
	}

	names := make([]string, 0, len(allNames))
	for name := range allNames {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		oldInp, inOld := old[name]
		newInp, inNew := new[name]

		if !inOld && inNew {
			fmt.Fprintf(b, "  %s+ input %q: type=%s%s\n", colorGreen, name, newInp.Type, colorReset)
		} else if inOld && !inNew {
			fmt.Fprintf(b, "  %s- input %q: type=%s%s\n", colorRed, name, oldInp.Type, colorReset)
		} else if oldInp.Type != newInp.Type || oldInp.Description != newInp.Description {
			fmt.Fprintf(b, "  %s~ input %q: type=%s → type=%s%s\n", colorYellow, name, oldInp.Type, newInp.Type, colorReset)
		}
	}
}

func diffSteps(b *strings.Builder, old, new []Step) {
	oldMap := make(map[string]Step)
	for _, s := range old {
		oldMap[s.Name] = s
	}
	newMap := make(map[string]Step)
	for _, s := range new {
		newMap[s.Name] = s
	}

	// Removed steps.
	for _, s := range old {
		if _, exists := newMap[s.Name]; !exists {
			fmt.Fprintf(b, "  %s- step %q: action=%s%s\n", colorRed, s.Name, s.Action, colorReset)
		}
	}

	// Added and modified steps (in new order).
	for _, s := range new {
		oldStep, exists := oldMap[s.Name]
		if !exists {
			fmt.Fprintf(b, "  %s+ step %q: action=%s%s\n", colorGreen, s.Name, s.Action, colorReset)
			continue
		}
		diffStep(b, oldStep, s)
	}
}

func diffStep(b *strings.Builder, old, new Step) {
	var changes []string

	if old.Action != new.Action {
		changes = append(changes, fmt.Sprintf("action: %q → %q", old.Action, new.Action))
	}
	if old.If != new.If {
		changes = append(changes, fmt.Sprintf("if: %q → %q", old.If, new.If))
	}
	if old.Timeout != new.Timeout {
		changes = append(changes, fmt.Sprintf("timeout: %q → %q", old.Timeout, new.Timeout))
	}
	if fmt.Sprintf("%v", old.Params) != fmt.Sprintf("%v", new.Params) {
		changes = append(changes, "params changed")
	}
	if fmt.Sprintf("%v", old.Retry) != fmt.Sprintf("%v", new.Retry) {
		changes = append(changes, "retry changed")
	}

	if len(changes) > 0 {
		fmt.Fprintf(b, "  %s~ step %q: %s%s\n", colorYellow, new.Name, strings.Join(changes, ", "), colorReset)
	}
}
