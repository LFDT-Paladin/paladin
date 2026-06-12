// Copyright © 2026 Kaleido, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
// an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
// specific language governing permissions and limitations under the License.
//
// SPDX-License-Identifier: Apache-2.0

// gendocs generates Mermaid state machine diagrams from Go state machine source files.
//
// State descriptions are loaded from en_states.go files co-located with each
// state_machine.go via go/ast. Transitions are extracted from stateDefinitionsMap.
//
// Regenerates:
//
//	doc-site/docs/architecture/distributed_sequencer_state_machine.md
//	doc-site/docs/architecture/distributed_sequencer_state_machine_transitions.md
//
// Run from anywhere within the repo:
//
//	cd scripts/gendocs && go run .
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const mermaidInit = "%%{init: {'themeVariables': {'background': 'transparent'}}}%%"

type smFile struct {
	name    string
	relPath string
}

var smFiles = []smFile{
	{"Coordinator", "core/go/internal/sequencer/coordinator/state_machine.go"},
	{"Coordinator Transaction", "core/go/internal/sequencer/coordinator/transaction/state_machine.go"},
	{"Originator", "core/go/internal/sequencer/originator/state_machine.go"},
	{"Originator Transaction", "core/go/internal/sequencer/originator/transaction/state_machine.go"},
}

type transition struct {
	fromState string
	event     string
	toState   string
	guard     string
}

type smData struct {
	name        string
	stateDescs  map[string]string
	stateOrder  []string
	transitions []transition
}

func main() {
	log.SetFlags(0)
	repoRoot := findRepoRoot()

	var allData []smData
	for _, f := range smFiles {
		path := filepath.Join(repoRoot, filepath.FromSlash(f.relPath))
		data, err := parseFile(path, f.name)
		if err != nil {
			log.Fatalf("ERROR: %v", err)
		}
		log.Printf("Parsed %s: %d states, %d transitions",
			f.name, len(data.stateOrder), len(data.transitions))
		allData = append(allData, data)
	}

	docPath := filepath.Join(repoRoot, "doc-site/docs/architecture/distributed_sequencer_state_machine.md")
	if err := os.WriteFile(docPath, []byte(generateDoc(allData)), 0644); err != nil {
		log.Fatalf("ERROR writing %s: %v", docPath, err)
	}
	log.Printf("Wrote %s", docPath)

	transPath := filepath.Join(repoRoot, "doc-site/docs/architecture/distributed_sequencer_state_machine_transitions.md")
	if err := os.WriteFile(transPath, []byte(generateTransitionsDoc(allData)), 0644); err != nil {
		log.Fatalf("ERROR writing %s: %v", transPath, err)
	}
	log.Printf("Wrote %s", transPath)
}

func findRepoRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			log.Fatal("ERROR: could not find repo root (go.work not found); run from within the repo")
		}
		dir = parent
	}
}

// parseFile parses a state_machine.go and its co-located en_states.go.
func parseFile(path, name string) (smData, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return smData{}, fmt.Errorf("parsing %s: %w", path, err)
	}

	stateOrder := parseConstStates(f)

	statesPath := filepath.Join(filepath.Dir(path), "en_states.go")
	stateDescs, err := parseEnStates(statesPath)
	if err != nil {
		return smData{}, fmt.Errorf("parsing %s: %w", statesPath, err)
	}

	return smData{
		name:        name,
		stateDescs:  stateDescs,
		stateOrder:  stateOrder,
		transitions: parseTransitions(f),
	}, nil
}

// parseConstStates returns State_* constant names in source order from the file's const blocks.
func parseConstStates(f *ast.File) []string {
	var states []string
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range vs.Names {
				if strings.HasPrefix(name.Name, "State_") {
					states = append(states, name.Name)
				}
			}
		}
	}
	return states
}

// parseEnStates parses an en_states.go file and returns a map from State_* name to translation.
// It finds pdm("pkg.State_Name", "translation") call expressions and extracts the state name
// (the part after the last ".") and its translation string.
func parseEnStates(path string) (map[string]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, err
	}
	msgs := make(map[string]string)
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		fn, ok := call.Fun.(*ast.Ident)
		if !ok || fn.Name != "pdm" || len(call.Args) != 2 {
			return true
		}
		keyLit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || keyLit.Kind != token.STRING {
			return true
		}
		valLit, ok := call.Args[1].(*ast.BasicLit)
		if !ok || valLit.Kind != token.STRING {
			return true
		}
		key := strings.Trim(keyLit.Value, `"`)
		val := strings.Trim(valLit.Value, `"`)
		// Extract the State_* portion after the last "."
		if dot := strings.LastIndex(key, "."); dot >= 0 {
			key = key[dot+1:]
		}
		if strings.HasPrefix(key, "State_") {
			msgs[key] = val
		}
		return true
	})
	return msgs, nil
}

func parseTransitions(f *ast.File) []transition {
	var result []transition
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		if len(vs.Names) != 1 || vs.Names[0].Name != "stateDefinitionsMap" {
			return true
		}
		if len(vs.Values) != 1 {
			return false
		}
		mapCL, ok := vs.Values[0].(*ast.CompositeLit)
		if !ok {
			return false
		}
		for _, elt := range mapCL.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			stateName := exprName(kv.Key)
			if !strings.HasPrefix(stateName, "State_") {
				continue
			}
			stateDef, ok := kv.Value.(*ast.CompositeLit)
			if !ok {
				continue
			}
			result = append(result, transitionsFromStateDef(stateName, stateDef)...)
		}
		return false
	})
	return result
}

func transitionsFromStateDef(stateName string, stateDef *ast.CompositeLit) []transition {
	var result []transition
	for _, elt := range stateDef.Elts {
		skv, ok := elt.(*ast.KeyValueExpr)
		if !ok || exprName(skv.Key) != "Events" {
			continue
		}
		eventsMap, ok := skv.Value.(*ast.CompositeLit)
		if !ok {
			continue
		}
		for _, eelt := range eventsMap.Elts {
			ekv, ok := eelt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			eventName := exprName(ekv.Key)
			if !strings.HasPrefix(eventName, "Event_") {
				continue
			}
			handlersCL, ok := ekv.Value.(*ast.CompositeLit)
			if !ok {
				continue
			}
			result = append(result, transitionsFromHandlers(stateName, eventName, handlersCL)...)
		}
	}
	return result
}

func transitionsFromHandlers(stateName, eventName string, handlersCL *ast.CompositeLit) []transition {
	var result []transition
	for _, helt := range handlersCL.Elts {
		hkv, ok := helt.(*ast.KeyValueExpr)
		if !ok || exprName(hkv.Key) != "Handlers" {
			continue
		}
		handlerList, ok := hkv.Value.(*ast.CompositeLit)
		if !ok {
			continue
		}
		for _, handler := range handlerList.Elts {
			handlerCL, ok := handler.(*ast.CompositeLit)
			if !ok {
				continue
			}
			result = append(result, transitionsFromHandler(stateName, eventName, handlerCL)...)
		}
	}
	return result
}

func transitionsFromHandler(stateName, eventName string, handlerCL *ast.CompositeLit) []transition {
	var result []transition
	for _, telt := range handlerCL.Elts {
		tkv, ok := telt.(*ast.KeyValueExpr)
		if !ok || exprName(tkv.Key) != "Transitions" {
			continue
		}
		transList, ok := tkv.Value.(*ast.CompositeLit)
		if !ok {
			continue
		}
		for _, trans := range transList.Elts {
			transCL, ok := trans.(*ast.CompositeLit)
			if !ok {
				continue
			}
			var toState, guard string
			for _, tfield := range transCL.Elts {
				tfkv, ok := tfield.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				switch exprName(tfkv.Key) {
				case "To":
					toState = exprName(tfkv.Value)
				case "If":
					guard = simplifyGuard(tfkv.Value)
				}
			}
			if strings.HasPrefix(toState, "State_") {
				result = append(result, transition{
					fromState: stateName,
					event:     eventName,
					toState:   toState,
					guard:     guard,
				})
			}
		}
	}
	return result
}

func exprName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	}
	return ""
}

func simplifyGuard(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return strings.TrimPrefix(e.Name, "guard_")
	case *ast.SelectorExpr:
		return strings.TrimPrefix(e.Sel.Name, "guard_")
	case *ast.CallExpr:
		fn := callFuncName(e)
		switch fn {
		case "GuardNot", "guard_Not":
			if len(e.Args) == 1 {
				return "!" + simplifyGuard(e.Args[0])
			}
		case "GuardAnd":
			parts := make([]string, len(e.Args))
			for i, a := range e.Args {
				parts[i] = simplifyGuard(a)
			}
			return strings.Join(parts, " && ")
		case "GuardOr":
			parts := make([]string, len(e.Args))
			for i, a := range e.Args {
				parts[i] = simplifyGuard(a)
			}
			return strings.Join(parts, " || ")
		}
		return fn
	}
	return ""
}

func callFuncName(e *ast.CallExpr) string {
	switch f := e.Fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}

// Mermaid generation

func mermaidID(stateName string) string {
	return strings.TrimPrefix(stateName, "State_")
}

func eventLabel(eventName string) string {
	return strings.TrimPrefix(eventName, "Event_")
}

func mermaidStateLabels(data smData) []string {
	var lines []string
	for _, s := range data.stateOrder {
		mid := mermaidID(s)
		label := strings.ReplaceAll(mid, "_", " ")
		if label != mid {
			lines = append(lines, fmt.Sprintf("    state %q as %s", label, mid))
		}
	}
	return lines
}

func terminalStates(data smData) []string {
	source := make(map[string]bool)
	for _, t := range data.transitions {
		source[t.fromState] = true
	}
	var result []string
	for _, s := range data.stateOrder {
		if !source[s] {
			result = append(result, s)
		}
	}
	return result
}

func generateMermaidSimple(data smData) string {
	var sb strings.Builder
	sb.WriteString(mermaidInit + "\n")
	sb.WriteString("stateDiagram-v2\n")
	sb.WriteString("    direction LR\n")
	for _, line := range mermaidStateLabels(data) {
		sb.WriteString(line + "\n")
	}
	if len(data.stateOrder) > 0 {
		fmt.Fprintf(&sb, "    [*] --> %s\n", mermaidID(data.stateOrder[0]))
	}
	seen := make(map[[2]string]bool)
	for _, t := range data.transitions {
		pair := [2]string{t.fromState, t.toState}
		if !seen[pair] {
			seen[pair] = true
			fmt.Fprintf(&sb, "    %s --> %s\n", mermaidID(t.fromState), mermaidID(t.toState))
		}
	}
	for _, s := range terminalStates(data) {
		fmt.Fprintf(&sb, "    %s --> [*]\n", mermaidID(s))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func generateMermaid(data smData) string {
	var sb strings.Builder
	sb.WriteString(mermaidInit + "\n")
	sb.WriteString("stateDiagram-v2\n")
	sb.WriteString("    direction LR\n")
	for _, line := range mermaidStateLabels(data) {
		sb.WriteString(line + "\n")
	}
	if len(data.stateOrder) > 0 {
		fmt.Fprintf(&sb, "    [*] --> %s\n", mermaidID(data.stateOrder[0]))
	}
	for _, t := range data.transitions {
		ev := eventLabel(t.event)
		label := ev
		if t.guard != "" {
			label = ev + " [" + t.guard + "]"
		}
		fmt.Fprintf(&sb, "    %s --> %s : %s\n", mermaidID(t.fromState), mermaidID(t.toState), label)
	}
	for _, s := range terminalStates(data) {
		fmt.Fprintf(&sb, "    %s --> [*]\n", mermaidID(s))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// Markdown generation

func generateStatesTable(data smData) string {
	var sb strings.Builder
	sb.WriteString("| State | Description |\n")
	sb.WriteString("| --- | --- |\n")
	for _, s := range data.stateOrder {
		label := strings.ReplaceAll(mermaidID(s), "_", " ")
		fmt.Fprintf(&sb, "| **%s** | %s |\n", label, data.stateDescs[s])
	}
	return strings.TrimRight(sb.String(), "\n")
}

func generateEventsTable(data smData) string {
	seen := make(map[string]bool)
	var events []string
	for _, t := range data.transitions {
		if !seen[t.event] {
			seen[t.event] = true
			events = append(events, t.event)
		}
	}
	sort.Strings(events)

	var sb strings.Builder
	sb.WriteString("| Event | Description |\n")
	sb.WriteString("| --- | --- |\n")
	for _, e := range events {
		fmt.Fprintf(&sb, "| **%s** | |\n", eventLabel(e))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func generateOverviewSection(data smData) string {
	return fmt.Sprintf("## %s State Machine\n\n```mermaid\n%s\n```\n\n### States\n\n%s\n",
		data.name, generateMermaidSimple(data), generateStatesTable(data))
}

func generateDetailSection(data smData) string {
	return fmt.Sprintf("## %s State Machine\n\n```mermaid\n%s\n```\n\n### Transition Events\n\n%s\n",
		data.name, generateMermaid(data), generateEventsTable(data))
}

func generateDoc(allData []smData) string {
	header := "# Sequencer and transaction state machines\n\n" +
		"The distributed sequencer is designed as a set of state machines, each of" +
		" which manages the state of the sequencer components (originator and" +
		" coordinator) and of sequencer transactions (at the originator and at" +
		" the coordinator).\n\n" +
		"*Auto-generated from source*\n\n"
	sections := make([]string, len(allData))
	for i, d := range allData {
		sections[i] = generateOverviewSection(d)
	}
	return header + strings.Join(sections, "\n---\n\n")
}

func generateTransitionsDoc(allData []smData) string {
	header := "# State machine transition detail\n\n" +
		"Detailed state diagrams showing every transition event and guard condition" +
		" for each of the four distributed sequencer state machines.\n\n" +
		"*Auto-generated from source*\n\n"
	sections := make([]string, len(allData))
	for i, d := range allData {
		sections[i] = generateDetailSection(d)
	}
	return header + strings.Join(sections, "\n---\n\n")
}
