# Gas City Formula

Schema for *.formula.toml — a formula definition file.

> **Auto-generated** — do not edit. Run `go run ./cmd/genschema` to regenerate.

## Formula

Formula is a parsed formula definition from a *.formula.toml file.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `formula` | string | **yes** |  | Name is the unique identifier for this formula. |
| `description` | string |  |  | Description explains what this formula does. |
| `steps` | []Step | **yes** |  | Steps defines the ordered sequence of work items in this formula. |

## Step

Step is one step in a formula.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `id` | string | **yes** |  | ID is the unique identifier for this step within the formula. |
| `title` | string | **yes** |  | Title is a short human-readable label for this step. |
| `description` | string |  |  | Description provides detailed instructions for this step. |
| `needs` | []string |  |  | Needs lists step IDs that must complete before this step can start. |

