# Task: Write Gas City Subsystem Documentation

You are writing a current-state architecture document for the
`{subsystem_name}` subsystem of Gas City.

## Context You Need

Read these files BEFORE writing:

1. **CLAUDE.md** (project root) — design principles, layering
   invariants, code conventions, the nine concepts
2. **docs/architecture/README.md** — how this doc fits into the
   overall architecture documentation
3. **docs/architecture/glossary.md** — use these term definitions
   exactly; propose additions if needed
4. **The implementation code:**
   - `internal/{package}/` — all `.go` files
   - `internal/{package}/*_test.go` — tests reveal intent
   - `internal/{package}/doc.go` — if it exists
5. **Any existing design docs** in `docs/design/` that proposed
   this subsystem

## Template

Use the template at `docs/architecture/TEMPLATE.md`. Fill in every
section. Do not skip sections — write "None" or "N/A" if a section
genuinely doesn't apply.

## What to Include

- **Concrete code references.** Name specific files, types, and
  functions. Use paths relative to the repo root:
  `internal/beads/store.go`.
- **Stated invariants.** These are the most valuable part of the
  document. Think about: what must be true for correctness? What
  can never happen? What ordering constraints exist?
- **Honest limitations.** Document what's missing, fragile, or
  planned-but-not-built. Don't paper over gaps.
- **Cross-references with context.** Don't just link to a file —
  explain why the reader would follow that link.

## What to Avoid

- **Don't describe aspirational state.** This is a "how it works
  now" document. If something isn't implemented, say so in Known
  Limitations, or write a design doc instead.
- **Don't repeat CLAUDE.md.** Reference it for design principles;
  don't copy them into every subsystem doc.
- **Don't include code blocks longer than 20 lines.** Link to the
  source file instead. Short snippets (under 10 lines) showing key
  interfaces or type definitions are fine.
- **Don't use vague language.** "The system handles errors
  appropriately" is useless. "Errors are wrapped with
  `fmt.Errorf("creating bead: %w", err)` and returned to the
  caller" is useful.
- **No role names in examples.** Gas City has zero hardcoded roles.
  Use generic names like "agent-1" or "the coding agent" in examples.

## Quality Checklist

Before submitting, verify:

- [ ] Summary is 2-4 sentences and self-contained
- [ ] All Key Concepts terms match the glossary
- [ ] Invariants are stated as testable assertions
- [ ] Code Map lists actual files that exist in the repo
- [ ] Cross-references use descriptive link text
- [ ] No role names appear (mayor, deacon, polecat, etc.)
- [ ] Document accurately reflects current code, not spec aspirations
