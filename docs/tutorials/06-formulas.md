# Tutorial 06 — Formulas: Don't Restart from Scratch

## The problem

You create a bead: "Set up CI pipeline." Your agent gets to work — installs
dependencies, configures the build, sets up test runners, and is halfway
through writing deployment config when its context window fills up and the
session compacts.

Fresh session. The agent reads the bead: "Set up CI pipeline." Status: open.
It starts from scratch — reinstalling dependencies it already installed,
re-reading files it already read. All progress lost.

A bead is all-or-nothing. It's either open or closed. There's no way to say
"I finished steps 1-3, pick up at step 4."

## The solution: formulas

A **formula** is a TOML file that breaks work into named steps with
dependencies. A **molecule** is a formula instantiated as real beads — a
root bead plus one child bead per step. Each step is a persistent
checkpoint. Close a step and it stays closed, even if the session dies.

On restart, query the molecule. The runtime tells you which step is
current — you don't have to figure it out.

## Defining a formula

Create a `.formula.toml` file in your city's formula directory:

```bash
mkdir -p .gc/formulas
```

Here's a formula for making pancakes (formulas work for any structured
process — not just code):

```toml
# .gc/formulas/pancakes.formula.toml
formula = "pancakes"
description = "Make a batch of fluffy pancakes"

[[steps]]
id = "dry"
title = "Mix dry ingredients"
description = """
Combine in a large bowl:
- 2 cups flour
- 2 tbsp sugar
- 1 tsp baking powder
- 1/2 tsp salt
Whisk together until evenly distributed."""

[[steps]]
id = "wet"
title = "Mix wet ingredients"
description = """
In a separate bowl, whisk together:
- 2 eggs
- 1.5 cups milk
- 1/4 cup melted butter
Beat until smooth."""
needs = ["dry"]

[[steps]]
id = "combine"
title = "Combine wet and dry"
description = """
Pour the wet ingredients into the dry ingredients.
Stir until just combined — lumps are OK! Overmixing
makes tough pancakes."""
needs = ["wet"]

[[steps]]
id = "cook"
title = "Cook the pancakes"
description = """
Heat griddle to 375F. Pour 1/4 cup batter per pancake.
Cook until bubbles form on surface and edges look set
(about 2 minutes), then flip. Cook 1-2 more minutes."""
needs = ["combine"]

[[steps]]
id = "serve"
title = "Serve"
description = "Stack pancakes, add butter and maple syrup."
needs = ["cook"]
```

A formula is a checklist, not a program. No conditionals, no loops, no retry
logic. If a step needs intelligence, that's the agent's job — the formula
just tracks which steps are done.

### Steps and dependencies

Each step declares what it `needs` — which steps must be closed before it
becomes ready. The runtime resolves dependencies and tells the agent which
step to work on next. The agent never has to scan a list and figure out
ordering — **Go finds the step, the agent executes it.**

### Listing formulas

```bash
$ gc formula list
NAME       STEPS  DESCRIPTION
pancakes   5      Make a batch of fluffy pancakes
```

```bash
$ gc formula show pancakes
Formula: pancakes
Description: Make a batch of fluffy pancakes

  STEP      TITLE                   NEEDS
  dry       Mix dry ingredients     —
  wet       Mix wet ingredients     dry
  combine   Combine wet and dry     wet
  cook      Cook the pancakes       combine
  serve     Serve                   cook
```

## Creating a molecule

A molecule is a live instance of a formula — real beads you can close:

```bash
$ gc mol create pancakes
Created molecule gc-5 from 'pancakes' (5 steps)
```

This creates six beads: one root (type "molecule") and five children (type
"step"), one per formula step. Child step beads are wired with dependencies
matching the formula's `needs`. All step beads start with status "open".

You can hook the molecule to an agent like any other bead:

```bash
$ gc agent hook mayor gc-5
Hooked gc-5 to mayor
```

## Working through steps

Query the molecule to find your current step:

```bash
$ gc mol status gc-5
MOLECULE  gc-5  pancakes  (0/5 complete)

Current step: dry — Mix dry ingredients

  Combine in a large bowl:
  - 2 cups flour
  - 2 tbsp sugar
  - 1 tsp baking powder
  - 1/2 tsp salt
  Whisk together until evenly distributed.

When done: gc mol step done gc-5 dry
```

The runtime identifies the current step and shows its full description.
The agent doesn't need to reason about step ordering — it just executes
what's shown and runs the completion command.

Complete a step when the work is done:

```bash
$ gc mol step done gc-5 dry
✓ Step 1/5: dry — Mix dry ingredients

Current step: wet — Mix wet ingredients

  In a separate bowl, whisk together:
  - 2 eggs
  - 1.5 cups milk
  - 1/4 cup melted butter
  Beat until smooth.

When done: gc mol step done gc-5 wet
```

Each `step done` closes the completed step and immediately shows the next
one. The agent's workflow is a simple loop: read the current step, do it,
mark it done.

## The crash recovery payoff

This is why formulas exist. Your agent has completed three steps when its
context compacts. Fresh session — the agent runs `gc mol status`:

```bash
$ gc mol status gc-5
MOLECULE  gc-5  pancakes  (3/5 complete)

Current step: cook — Cook the pancakes

  Heat griddle to 375F. Pour 1/4 cup batter per pancake.
  Cook until bubbles form on surface and edges look set
  (about 2 minutes), then flip. Cook 1-2 more minutes.

When done: gc mol step done gc-5 cook
```

Three steps done, "cook" is current. The agent picks up exactly where it
left off. No wasted work. No re-reading. No guessing.

```bash
$ gc mol step done gc-5 cook
✓ Step 4/5: cook — Cook the pancakes

Current step: serve — Serve

  Stack pancakes, add butter and maple syrup.

When done: gc mol step done gc-5 serve

$ gc mol step done gc-5 serve
✓ Step 5/5: serve — Serve
All steps complete. Molecule gc-5 closed.
```

When the last step closes, the molecule root bead closes automatically.

## How it works underneath

When `gc mol create pancakes` runs:

1. Parse `pancakes.formula.toml` — validate step IDs, check that all
   `needs` references exist, detect cycles.
2. Create the root bead: `{Type: "molecule", Title: "pancakes"}`.
3. For each step, create a child bead: `{Type: "step", ParentID: root.ID,
   Ref: step.id, Title: step.title}`. The step's `needs` are stored so
   the runtime can resolve dependencies later.
4. All step beads start with status "open".

When `gc mol status gc-5` runs:

1. Load the root bead and all children (by ParentID).
2. For each child, check its status and whether its `needs` are all closed.
3. The first step whose needs are satisfied and is still open = current.
4. Print the current step with its full description.

When `gc mol step done gc-5 dry` runs:

1. Close the "dry" step bead.
2. Re-evaluate: find the next step whose needs are now satisfied.
3. Print that step's description. Or if all steps are closed, close the
   molecule root and print completion.

The molecule IS the state. No external tracking, no session memory. Beads
persist; sessions come and go.

## A code example

Formulas aren't just for recipes. Here's a realistic feature implementation:

```toml
# .gc/formulas/implement.formula.toml
formula = "implement"
description = "Implement a feature with checkpointed progress"

[[steps]]
id = "understand"
title = "Read and understand the requirement"
description = """
Read the bead description. Identify affected files and packages.
Note any open questions or ambiguities. If blocked, ask before
proceeding."""

[[steps]]
id = "design"
title = "Design the approach"
description = """
Decide on the implementation strategy. List files to create or
change. Identify edge cases. Keep it simple — don't gold-plate."""
needs = ["understand"]

[[steps]]
id = "implement"
title = "Write the code"
description = """
Implement the solution. Make atomic commits as you go. Follow
existing codebase conventions."""
needs = ["design"]

[[steps]]
id = "test"
title = "Run tests"
description = """
Run the full test suite. Fix any failures your changes introduced.
Add tests for new code paths."""
needs = ["implement"]

[[steps]]
id = "review"
title = "Self-review and clean up"
description = """
Review your diff. Remove debug code. Ensure commit messages are
clear. Verify no unintended changes."""
needs = ["test"]
```

An agent can finish "understand" and "design", crash, restart, and jump
straight to "implement" — the thinking work isn't lost.

## What formulas are NOT

Formulas are a checklist. They don't contain:

- **Conditionals** — no `if test fails then ...`. The agent decides.
- **Loops** — no `retry 3 times`. The agent decides.
- **Timeouts** — no `max 5 minutes`. The agent decides.
- **Logic** — no framework intelligence. The agent reads the step
  description and acts.

This is ZFC: Go tracks which boxes are checked. The agent decides what
checking a box means.

## Config

Add the formulas directory to your city config:

```toml
# city.toml
[formulas]
dir = ".gc/formulas"
```

If omitted, defaults to `.gc/formulas`.

## Commands reference

```bash
gc formula list                     # List available formulas
gc formula show <name>              # Show formula steps and dependencies

gc mol create <formula>             # Instantiate formula as molecule
gc mol list                         # List all molecules
gc mol status <mol-id>              # Show current step with description
gc mol step done <mol-id> <step>    # Complete a step, show next
```

## What's next

With formulas, agents can work through multi-step processes and survive
context loss. The work is the state — not the session, not the memory.

This is the MEOW stack in action: beads track individual work units,
molecules compose them into structured workflows, and the formula defines
the template.

Next tutorials add health monitoring (agents that crash get restarted
automatically) and plugins (formulas triggered by events rather than
humans).
