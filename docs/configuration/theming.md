# Theming

Perles supports comprehensive theming with built-in presets and customizable color tokens.

## Quick Start

Use a built-in theme preset:

```yaml
theme:
  preset: catppuccin-mocha
```

---

## Available Presets

Run `perles themes` to see all available presets:

| Preset | Description |
|--------|-------------|
| `default` | Default perles theme |
| `catppuccin-mocha` | Warm, cozy dark theme |
| `catppuccin-latte` | Warm, cozy light theme |
| `dracula` | Dark theme with vibrant colors |
| `nord` | Arctic, north-bluish palette |
| `high-contrast` | High contrast for accessibility |
| `gruvbox` | Retro groove color scheme |

For a light terminal, pick a light preset such as `catppuccin-latte`. Rendered markdown in the issue details pane is styled separately, by `ui.markdown_style` (`dark` or `light`; see [Configuration](index.md)). The `theme.mode` setting is ignored: it is still accepted for older configs, but perles does not switch between light and dark variants.

---

## Customizing Colors

Override specific colors while using a preset:

```yaml
theme:
  preset: dracula
  colors:
    status.error: "#FF0000"
    priority.critical: "#FF5555"
```

Or create a fully custom theme from scratch:

```yaml
theme:
  colors:
    text.primary: "#FFFFFF"
    text.muted: "#888888"
    status.success: "#00FF00"
    status.error: "#FF0000"
    border.default: "#444444"
    border.highlight: "#FFFFFF"
```

Tokens you don't set keep the value from the preset, or from the `default` preset if no preset is set.

Token names can be written as dotted keys (as above) or as nested YAML (`text:` then `primary:`). Color values must be quoted hex strings, `"#RGB"` or `"#RRGGBB"`. Named colors and ANSI color numbers are not supported. Without the quotes, YAML reads `#FF0000` as a comment and the override is dropped.

Perles checks the preset name, every token name, and every color value before applying any of them. If any one is invalid, the whole `theme` section (preset included) is ignored and the default theme is used. Startup continues and shows a warning toast such as `Theme config ignored: unknown color token: text.primry`. The other possible errors are `unknown theme preset: <name>` and `invalid hex color for <token>: <value>`.

To see every token's value in each preset, run `perles playground` and open the Theme Tokens demo.

---

## Color Tokens

Colors are organized by category:

### Text

| Token | Description |
|-------|-------------|
| `text.primary` | Primary text color |
| `text.secondary` | Secondary text color |
| `text.muted` | Muted/dimmed text |
| `text.description` | Description text |
| `text.placeholder` | Placeholder text |

### Border

| Token | Description |
|-------|-------------|
| `border.default` | Default (unfocused) border and divider color |
| `border.highlight` | Focused elements: the border of the focused pane, input, or form field, active tab labels, and the dashboard's active-filter indicator |
| `border.focus` | Alias for `border.highlight`. Used only when you don't also set `border.highlight`; the `border.focus` values in built-in presets have no effect |

### Status

| Token | Description |
|-------|-------------|
| `status.success` | Success state |
| `status.warning` | Warning state |
| `status.error` | Error state |

### Buttons

| Token | Description |
|-------|-------------|
| `button.text` | Button text color |
| `button.primary.bg` | Primary button background |
| `button.primary.focus` | Primary button focus state |
| `button.secondary.bg` | Secondary button background |
| `button.secondary.focus` | Secondary button focus state |
| `button.danger.bg` | Danger button background |
| `button.danger.focus` | Danger button focus state |
| `button.disabled.bg` | Disabled button background |

### Selection

| Token | Description |
|-------|-------------|
| `selection.indicator` | Selection indicator |
| `selection.background` | Selection background |

### Overlays

| Token | Description |
|-------|-------------|
| `overlay.title` | Titles of modals, overlays, and pane headers |
| `overlay.border` | Borders and dividers of modals, pickers, and overlays |

### Toasts

| Token | Description |
|-------|-------------|
| `toast.success` | Success toast |
| `toast.error` | Error toast |
| `toast.info` | Info toast |
| `toast.warn` | Warning toast |

### Issue Priority

| Token | Description |
|-------|-------------|
| `priority.critical` | P0 critical priority |
| `priority.high` | P1 high priority |
| `priority.medium` | P2 medium priority |
| `priority.low` | P3 low priority |
| `priority.backlog` | P4 backlog priority |

### Issue Status

| Token | Description |
|-------|-------------|
| `issue.status.open` | Open issue color |
| `issue.status.in_progress` | In-progress issue color |
| `issue.status.closed` | Closed issue color |
| `issue.status.deferred` | Deferred issue color |
| `issue.status.blocked` | Blocked issue color |

### Issue Type

| Token | Description |
|-------|-------------|
| `type.task` | Task type color |
| `type.bug` | Bug type color |
| `type.feature` | Feature type color |
| `type.epic` | Epic type color |
| `type.chore` | Chore type color |
| `type.milestone` | Milestone type color |
| `type.story` | Story type color |
| `type.spike` | Spike type color |
| `type.molecule` | Molecule type color |
| `type.convoy` | Convoy type color |
| `type.agent` | Agent type color |

### BQL Syntax Highlighting

| Token | Description |
|-------|-------------|
| `bql.keyword` | BQL keywords (and, or, not) |
| `bql.operator` | BQL operators (=, !=, ~) |
| `bql.field` | BQL field names |
| `bql.string` | BQL string values |
| `bql.literal` | BQL literal values |
| `bql.paren` | BQL parentheses |
| `bql.comma` | BQL commas |

### Misc

| Token | Description |
|-------|-------------|
| `spinner` | Loading spinner |

### Accepted but Not Yet Applied

These tokens pass validation but currently change nothing on screen:

- `form.border`, `form.border.focus`, `form.label`, `form.label.focus` (focused form fields use `border.highlight`)
- `diff.addition`, `diff.deletion`, `diff.context`, `diff.hunk`
