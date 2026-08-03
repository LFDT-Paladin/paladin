#!/usr/bin/env python3
"""
Generate Mermaid state machine diagrams from Go state machine source files.

Reads the 4 state_machine.go files for coordinator, coordinator/transaction,
originator, and originator/transaction and regenerates:
  doc-site/docs/architecture/distributed_sequencer_state_machine.md

Run from anywhere within the repo:
  python3 scripts/generate_state_machine_docs.py
"""

import re
import sys
from pathlib import Path
from dataclasses import dataclass, field
from typing import Optional, List, Dict, Tuple

# ---------------------------------------------------------------------------
# Paths
# ---------------------------------------------------------------------------

REPO_ROOT = Path(__file__).resolve().parent.parent

STATE_MACHINE_FILES: List[Tuple[str, str]] = [
    (
        "Coordinator",
        "core/go/internal/sequencer/coordinator/state_machine.go",
    ),
    (
        "Coordinator Transaction",
        "core/go/internal/sequencer/coordinator/transaction/state_machine.go",
    ),
    (
        "Originator",
        "core/go/internal/sequencer/originator/state_machine.go",
    ),
    (
        "Originator Transaction",
        "core/go/internal/sequencer/originator/transaction/state_machine.go",
    ),
]

DOC_OUTPUT = "doc-site/docs/architecture/distributed_sequencer_state_machine.md"
DOC_OUTPUT_TRANSITIONS = "doc-site/docs/architecture/distributed_sequencer_state_machine_transitions.md"

# ---------------------------------------------------------------------------
# Data types
# ---------------------------------------------------------------------------


@dataclass
class Transition:
    from_state: str
    event: str
    to_state: str
    guard: Optional[str] = None


@dataclass
class StateMachineData:
    name: str
    states: Dict[str, str]        # state_name -> description (from const comment)
    events: Dict[str, str]        # event_name -> description (from const comment)
    transitions: List[Transition]
    initial_state: Optional[str]  # first state in the const block


# ---------------------------------------------------------------------------
# Low-level helpers
# ---------------------------------------------------------------------------


def extract_block(content: str, brace_pos: int) -> str:
    """Return content[brace_pos .. matching-'}'] inclusive."""
    assert content[brace_pos] == "{", (
        f"Expected '{{' at pos {brace_pos}, got {content[brace_pos]!r}"
    )
    depth = 0
    for i in range(brace_pos, len(content)):
        ch = content[i]
        if ch == "{":
            depth += 1
        elif ch == "}":
            depth -= 1
            if depth == 0:
                return content[brace_pos : i + 1]
    return content[brace_pos:]


def find_const_block(content: str, marker: str) -> Optional[str]:
    """
    Find a `const (...)` block whose body contains marker.
    Uses paren-balancing so comments containing ')' don't break the parse.
    Returns the inner content (between the parens), or None.
    """
    for m in re.finditer(r"\bconst\s*\(", content):
        paren_pos = m.end() - 1  # position of '('
        depth = 0
        pos = paren_pos
        while pos < len(content):
            ch = content[pos]
            if ch == "(":
                depth += 1
            elif ch == ")":
                depth -= 1
                if depth == 0:
                    inner = content[paren_pos + 1 : pos]
                    if marker in inner:
                        return inner
                    break
            pos += 1
    return None


def extract_field_value(block: str, field_name: str) -> Optional[str]:
    """
    Find `field_name: <value>` inside a Go struct block and return <value>.
    Handles values that contain nested parentheses.
    """
    pattern = re.compile(rf"\b{re.escape(field_name)}:\s*")
    m = pattern.search(block)
    if not m:
        return None
    pos = m.end()
    depth = 0
    start = pos
    while pos < len(block):
        ch = block[pos]
        if ch == "(":
            depth += 1
        elif ch == ")":
            depth -= 1
        elif ch in ",\n" and depth == 0:
            break
        elif ch == "}" and depth == 0:
            break
        pos += 1
    return block[start:pos].strip()


def split_args(s: str) -> List[str]:
    """Split comma-separated arguments, ignoring commas inside parentheses."""
    parts: List[str] = []
    depth = 0
    cur: List[str] = []
    for ch in s:
        if ch == "(":
            depth += 1
        elif ch == ")":
            depth -= 1
        if ch == "," and depth == 0:
            parts.append("".join(cur).strip())
            cur = []
        else:
            cur.append(ch)
    if cur:
        parts.append("".join(cur).strip())
    return [p for p in parts if p]


# ---------------------------------------------------------------------------
# Guard simplification
# ---------------------------------------------------------------------------


def simplify_guard(expr: str) -> str:
    """Convert a Go guard expression to a compact human-readable string."""
    expr = expr.strip().rstrip(",").strip()

    # guard_Not(...) – local helper variant of GuardNot
    m = re.fullmatch(r"guard_Not\((.+)\)", expr)
    if m:
        return f"!{simplify_guard(m.group(1))}"

    # statemachine.GuardNot(...) or GuardNot(...)
    m = re.fullmatch(r"(?:statemachine\.)?GuardNot\((.+)\)", expr)
    if m:
        return f"!{simplify_guard(m.group(1))}"

    # statemachine.GuardAnd(...) or GuardAnd(...)
    m = re.fullmatch(r"(?:statemachine\.)?GuardAnd\((.+)\)", expr)
    if m:
        return " && ".join(simplify_guard(a) for a in split_args(m.group(1)))

    # statemachine.GuardOr(...) or GuardOr(...)
    m = re.fullmatch(r"(?:statemachine\.)?GuardOr\((.+)\)", expr)
    if m:
        return " || ".join(simplify_guard(a) for a in split_args(m.group(1)))

    # guard_Xxx -> strip the prefix
    m = re.fullmatch(r"guard_(\w+)", expr)
    if m:
        return m.group(1)

    return expr


# ---------------------------------------------------------------------------
# Parsing const blocks
# ---------------------------------------------------------------------------


def parse_states(content: str) -> Dict[str, str]:
    """Return {State_Xxx: description} from the const block containing State_ aliases."""
    states: Dict[str, str] = {}
    block = find_const_block(content, "State_")
    if not block:
        return states
    for line in block.splitlines():
        lm = re.match(r"\s*(State_\w+)", line)
        if lm:
            name = lm.group(1)
            cm = re.search(r"//+\s*(.+)", line)
            states[name] = cm.group(1).strip() if cm else ""
    return states


def parse_events(content: str) -> Dict[str, str]:
    """Return {Event_Xxx: description} from all `const (... EventType ...)` blocks."""
    events: Dict[str, str] = {}
    # Use find_const_block for each EventType block; there may be multiple
    # Scan manually since find_const_block only returns the first match.
    for m in re.finditer(r"\bconst\s*\(", content):
        paren_pos = m.end() - 1
        depth = 0
        pos = paren_pos
        while pos < len(content):
            ch = content[pos]
            if ch == "(":
                depth += 1
            elif ch == ")":
                depth -= 1
                if depth == 0:
                    inner = content[paren_pos + 1 : pos]
                    if "EventType" in inner:
                        for line in inner.splitlines():
                            lm = re.match(r"\s*(Event_\w+)", line)
                            if lm:
                                name = lm.group(1)
                                cm = re.search(r"//+\s*(.+)", line)
                                events[name] = cm.group(1).strip() if cm else ""
                    break
            pos += 1
    return events


# ---------------------------------------------------------------------------
# Parsing the stateDefinitionsMap
# ---------------------------------------------------------------------------


def parse_transition_structs(
    trans_block: str, from_state: str, event: str
) -> List[Transition]:
    """Parse a `[]Transition{...}` block and return all Transition objects."""
    result: List[Transition] = []
    pos = 1  # skip opening '{'
    n = len(trans_block)
    while pos < n:
        pos += len(re.match(r"\s*", trans_block[pos:]).group())
        if pos >= n or trans_block[pos] == "}":
            break
        if trans_block[pos] == "{":
            struct_block = extract_block(trans_block, pos)
            to_val = extract_field_value(struct_block, "To")
            if_val = extract_field_value(struct_block, "If")
            if to_val and re.match(r"State_\w+", to_val):
                result.append(
                    Transition(
                        from_state=from_state,
                        event=event,
                        to_state=to_val.strip(),
                        guard=simplify_guard(if_val) if if_val else None,
                    )
                )
            pos += len(struct_block)
        else:
            pos += 1
    return result


def parse_event_handlers(events_block: str, from_state: str) -> List[Transition]:
    """Parse a `map[EventType]EventHandler{...}` block, returning transitions."""
    result: List[Transition] = []
    pos = 1  # skip opening '{'
    n = len(events_block)
    while pos < n:
        pos += len(re.match(r"\s*", events_block[pos:]).group())
        if pos >= n or events_block[pos] == "}":
            break
        # Match event key: Event_Xxx or common.Event_Xxx
        em = re.match(r"(?:common\.)?(Event_\w+)\s*:", events_block[pos:])
        if em:
            event_name = em.group(1)
            pos += em.end()
            pos += len(re.match(r"\s*", events_block[pos:]).group())
            if pos < n and events_block[pos] == "{":
                handler_block = extract_block(events_block, pos)
                # Find Transitions: []Transition{
                tm = re.search(r"\bTransitions:\s*\[\]Transition\{", handler_block)
                if tm:
                    tb_start = tm.start() + len(tm.group()) - 1
                    trans_block = extract_block(handler_block, tb_start)
                    result.extend(
                        parse_transition_structs(trans_block, from_state, event_name)
                    )
                pos += len(handler_block)
        else:
            pos += 1
    return result


def parse_state_machine(file_path: Path, name: str) -> StateMachineData:
    """Parse a state_machine.go file and return a StateMachineData object."""
    content = file_path.read_text()

    states = parse_states(content)
    events = parse_events(content)
    transitions: List[Transition] = []

    map_marker = "var stateDefinitionsMap = StateDefinitions{"
    map_pos = content.find(map_marker)
    if map_pos == -1:
        print(f"  WARNING: no stateDefinitionsMap found in {file_path}", file=sys.stderr)
    else:
        map_block = extract_block(content, map_pos + len(map_marker) - 1)
        pos = 1  # skip opening '{'
        n = len(map_block)
        while pos < n:
            pos += len(re.match(r"\s*", map_block[pos:]).group())
            if pos >= n or map_block[pos] == "}":
                break
            # Match state key: State_Xxx:
            sm = re.match(r"(State_\w+)\s*:", map_block[pos:])
            if sm:
                state_name = sm.group(1)
                pos += sm.end()
                pos += len(re.match(r"\s*", map_block[pos:]).group())
                if pos < n and map_block[pos] == "{":
                    state_block = extract_block(map_block, pos)
                    # Find Events: map[EventType]EventHandler{
                    evm = re.search(
                        r"\bEvents:\s*map\[EventType\]EventHandlers?\{", state_block
                    )
                    if evm:
                        eb_start = evm.start() + len(evm.group()) - 1
                        events_block = extract_block(state_block, eb_start)
                        transitions.extend(
                            parse_event_handlers(events_block, state_name)
                        )
                    pos += len(state_block)
            else:
                pos += 1

    initial_state = next(iter(states), None)
    return StateMachineData(
        name=name,
        states=states,
        events=events,
        transitions=transitions,
        initial_state=initial_state,
    )


# ---------------------------------------------------------------------------
# Mermaid generation
# ---------------------------------------------------------------------------


def _mermaid_id(state_name: str) -> str:
    """Convert State_Xxx_Yyy to Xxx_Yyy (drop the 'State_' prefix)."""
    return state_name.removeprefix("State_")


def _event_label(event_name: str) -> str:
    """Convert Event_Xxx to Xxx."""
    return event_name.removeprefix("Event_")


def _mermaid_state_labels(data: StateMachineData) -> list:
    """Return `state "..." as ...` lines for multi-word state names."""
    lines = []
    for state_name in data.states:
        mid = _mermaid_id(state_name)
        label = mid.replace("_", " ")
        if label != mid:
            lines.append(f'    state "{label}" as {mid}')
    return lines


def _mermaid_terminal_states(data: StateMachineData) -> list:
    """Return state names that have no outgoing transitions."""
    source_states = {t.from_state for t in data.transitions}
    return [s for s in data.states if s not in source_states]


MERMAID_INIT = "%%{init: {'themeVariables': {'background': 'transparent'}}}%%"


def generate_mermaid_simple(data: StateMachineData) -> str:
    """Generate a high-level Mermaid stateDiagram-v2 showing only states and links."""
    lines = [MERMAID_INIT, "stateDiagram-v2", "    direction LR"]
    lines.extend(_mermaid_state_labels(data))

    if data.initial_state:
        lines.append(f"    [*] --> {_mermaid_id(data.initial_state)}")

    # Deduplicate: only one arrow per unique (from, to) pair
    seen: set = set()
    for t in data.transitions:
        pair = (t.from_state, t.to_state)
        if pair not in seen:
            seen.add(pair)
            lines.append(f"    {_mermaid_id(t.from_state)} --> {_mermaid_id(t.to_state)}")

    for s in _mermaid_terminal_states(data):
        lines.append(f"    {_mermaid_id(s)} --> [*]")

    return "\n".join(lines)


def generate_mermaid(data: StateMachineData) -> str:
    """Generate a detailed Mermaid stateDiagram-v2 including transition event labels."""
    lines = [MERMAID_INIT, "stateDiagram-v2", "    direction LR"]
    lines.extend(_mermaid_state_labels(data))

    if data.initial_state:
        lines.append(f"    [*] --> {_mermaid_id(data.initial_state)}")

    for t in data.transitions:
        event = _event_label(t.event)
        label = f"{event} [{t.guard}]" if t.guard else event
        from_id = _mermaid_id(t.from_state)
        to_id = _mermaid_id(t.to_state)
        lines.append(f"    {from_id} --> {to_id} : {label}")

    for s in _mermaid_terminal_states(data):
        lines.append(f"    {_mermaid_id(s)} --> [*]")

    return "\n".join(lines)


# ---------------------------------------------------------------------------
# Markdown generation
# ---------------------------------------------------------------------------


def generate_states_table(data: StateMachineData) -> str:
    rows = ["| State | Description |", "| --- | --- |"]
    for state_name, desc in data.states.items():
        label = _mermaid_id(state_name).replace("_", " ")
        rows.append(f"| **{label}** | {desc} |")
    return "\n".join(rows)


def generate_events_table(data: StateMachineData) -> str:
    used_events = {t.event for t in data.transitions}
    rows = ["| Event | Description |", "| --- | --- |"]
    for event_name, desc in data.events.items():
        if event_name in used_events:
            label = _event_label(event_name)
            rows.append(f"| **{label}** | {desc} |")
    # common.Event_XXX events present in transitions but not in local const blocks
    common_events = sorted(used_events - set(data.events))
    for event_name in common_events:
        label = _event_label(event_name)
        rows.append(f"| **{label}** | (shared event from sequencer common package) |")
    return "\n".join(rows)


def generate_overview_section(data: StateMachineData) -> str:
    mermaid_simple = generate_mermaid_simple(data)
    states_table = generate_states_table(data)

    return (
        f"## {data.name} State Machine\n\n"
        f"```mermaid\n{mermaid_simple}\n```\n\n"
        f"### States\n\n"
        f"{states_table}\n"
    )


def generate_detail_section(data: StateMachineData) -> str:
    mermaid_detailed = generate_mermaid(data)
    events_table = generate_events_table(data)

    return (
        f"## {data.name} State Machine\n\n"
        f"```mermaid\n{mermaid_detailed}\n```\n\n"
        f"### Transition Events\n\n"
        f"{events_table}\n"
    )


def generate_doc(all_data: List[StateMachineData]) -> str:
    header = (
        "# Sequencer and transaction state machines\n\n"
        "The distributed sequencer is designed as a set of state machines, each of"
        " which manages the state of the sequencer components (originator and"
        " coordinator) and of sequencer transactions (at the originator and at"
        " the coordinator).\n\n"
        "*Auto-generated from source*\n\n"
    )
    sections = "\n---\n\n".join(generate_overview_section(d) for d in all_data)
    return header + sections


def generate_transitions_doc(all_data: List[StateMachineData]) -> str:
    header = (
        "# State machine transition detail\n\n"
        "Detailed state diagrams showing every transition event and guard condition"
        " for each of the four distributed sequencer state machines.\n\n"
        "*Auto-generated from source*\n\n"
    )
    sections = "\n---\n\n".join(generate_detail_section(d) for d in all_data)
    return header + sections


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------


def main() -> None:
    all_data: List[StateMachineData] = []
    for name, rel_path in STATE_MACHINE_FILES:
        file_path = REPO_ROOT / rel_path
        if not file_path.exists():
            print(f"ERROR: {file_path} not found", file=sys.stderr)
            sys.exit(1)
        print(f"Parsing {name} ({rel_path}) ...")
        data = parse_state_machine(file_path, name)
        print(
            f"  {len(data.states)} states, "
            f"{len(data.events)} events, "
            f"{len(data.transitions)} transitions"
        )
        all_data.append(data)

    doc = generate_doc(all_data)
    out_path = REPO_ROOT / DOC_OUTPUT
    out_path.write_text(doc)
    print(f"\nWrote {out_path}")

    transitions_doc = generate_transitions_doc(all_data)
    transitions_path = REPO_ROOT / DOC_OUTPUT_TRANSITIONS
    transitions_path.write_text(transitions_doc)
    print(f"Wrote {transitions_path}")


if __name__ == "__main__":
    main()
