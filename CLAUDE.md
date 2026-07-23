@AGENTS.md
@docs/PROJECT_BRIEF.md
@docs/CURRENT_TASK.md
@docs/DECISIONS.md

# Claude Code Additions

- Use project subagents in `.claude/agents/` proactively when their descriptions match.
- Use plan mode for architecture, database schema, security boundaries, and major cross-cutting changes.
- Do not run implementation subagents concurrently on overlapping files.
- Built-in Explore/Plan agents are useful for discovery, but use the project reviewers for domain-specific checks.
- After a major stage, run `code-reviewer` and the relevant specialist reviewer before claiming completion.
- Commands are available in `.claude/commands/`.
