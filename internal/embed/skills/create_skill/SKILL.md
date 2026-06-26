---
name: create_skill
description: Learn how to create agent skills — file layout, SKILL.md frontmatter fields, body conventions, progressive disclosure, and discovery. Use when asked how to write or add a skill.
---

# Creating Agent Skills

Agent Skills are an open format (agentskills.io) for extending AI agents with specialized knowledge and workflows. A skill is a directory containing a `SKILL.md` file. Agents load skills progressively: only the name and description are read at startup; the full body loads only when the skill is activated.

## Directory structure

```
skill-name/
├── SKILL.md          # Required: frontmatter + instructions
├── scripts/          # Optional: executable code the agent can run
├── references/       # Optional: documentation loaded on demand
├── assets/           # Optional: templates, data files, images
└── ...               # Any other files or directories
```

The directory name must match the `name` field in the frontmatter exactly.

## `SKILL.md` frontmatter

```markdown
---
name: skill-name
description: What the skill does and when to use it.
---
```

Two fields are required:

- **`name`** — 1–64 chars. Lowercase letters, digits, and hyphens only. No leading, trailing, or consecutive hyphens. Must match the directory name exactly.
- **`description`** — 1–1024 chars. Describes what the skill does and when to use it. This is the only thing the agent sees before deciding to activate.

### Writing a good `description`

The description is the most important field. The agent reads it at startup to decide whether to activate the skill. Make it specific and trigger-oriented:

```yaml
# Good — states what it does and when
description: Extracts text and tables from PDF files, fills PDF forms, and merges multiple PDFs. Use when working with PDF documents or when the user mentions PDFs, forms, or document extraction.

# Poor — too vague
description: Helps with PDFs.
```

Valid names: `pdf-processing`, `data-analysis`, `code-review`
Invalid: `PDF-Processing` (uppercase), `-pdf` (leading hyphen), `pdf--processing` (consecutive hyphens)

## Body

Everything after the closing `---` is the skill body — plain markdown with no format restrictions. Write whatever helps the agent perform the task.

Recommended sections:
- Step-by-step instructions
- Input/output examples
- Common edge cases
- References to supplementary files (use relative paths from the skill root)

Keep `SKILL.md` under 500 lines (~5000 tokens). Move detailed reference material to separate files in `references/` and instruct the agent to load them when needed.

## Minimal example

```
.agents/skills/
  roll-dice/
    SKILL.md
```

```markdown
---
name: roll-dice
description: Roll dice using a random number generator. Use when asked to roll a die (d6, d20, etc.) or generate a random dice roll.
---

To roll a die, run:

    echo $((RANDOM % <sides> + 1))

Replace `<sides>` with the number of sides (e.g. 6 for d6, 20 for d20).
```

## Progressive disclosure

Skills load in three stages, from cheapest to most expensive:

1. **Metadata** (~100 tokens) — `name` + `description`, loaded for all skills at startup
2. **Instructions** (<5000 tokens recommended) — full `SKILL.md` body, loaded on activation
3. **Resources** (as needed) — files in `scripts/`, `references/`, `assets/`, loaded on demand

Structure your skill to take advantage of this: put just enough in the body to handle the common case, and reference supplementary files for edge cases or detailed lookup material.

## Discovery locations

Agents typically discover skills from:
- `.agents/skills/` walked up from the current working directory to the project root
- `~/.agents/skills/` for user-global skills

The first occurrence of a name wins, so project-level skills override user-level ones.

