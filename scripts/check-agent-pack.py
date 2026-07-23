from pathlib import Path
import json
import tomllib

root = Path(__file__).resolve().parents[1]

required = [
    "AGENTS.md",
    "CLAUDE.md",
    "MASTER_PROMPT.md",
    ".codex/config.toml",
    ".codex/agents/architect.toml",
    ".claude/agents/architect.md",
    ".kiro/agents/invoiceflow-orchestrator.json",
    ".kiro/steering/product.md",
    "docs/CURRENT_TASK.md",
]

missing = [p for p in required if not (root / p).is_file()]
if missing:
    raise SystemExit("Missing files:\n" + "\n".join(missing))

for p in (root / ".kiro/agents").glob("*.json"):
    data = json.loads(p.read_text(encoding="utf-8"))
    for key in ("name", "description", "tools", "resources", "prompt"):
        assert key in data, f"{p}: missing {key}"

for p in (root / ".codex/agents").glob("*.toml"):
    data = tomllib.loads(p.read_text(encoding="utf-8"))
    for key in ("name", "description", "developer_instructions"):
        assert key in data, f"{p}: missing {key}"

for p in (root / ".claude/agents").glob("*.md"):
    text = p.read_text(encoding="utf-8")
    assert text.startswith("---\n"), f"{p}: no frontmatter"
    assert "\ndescription:" in text, f"{p}: no description"

print("Agent pack validation passed.")
