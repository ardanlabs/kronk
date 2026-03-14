# BUI Components — Agent Guidelines

## Tooltip & Label Conventions

All parameter explanations use the **type-safe tooltip system** in `ParamTooltips.tsx`. Follow these rules when adding or modifying form fields, labels, or parameter descriptions.

### Core Types & Exports (`ParamTooltips.tsx`)

| Export | Purpose |
|---|---|
| `PARAM_TOOLTIPS` | Const object mapping tooltip keys → explanation strings. Typed with `as const satisfies Record<string, string>`. |
| `TooltipKey` | `keyof typeof PARAM_TOOLTIPS` — use this type everywhere instead of `string` for tooltip keys. |
| `ParamTooltip` | Inline ⓘ icon + hover tooltip. Accepts **either** `tooltipKey: TooltipKey` (shared) **or** `text: string` (one-off). Never both. |
| `labelWithTip` | `(label: string, tooltipKey: TooltipKey) => ReactNode` — for table rows and `ReactNode` labels. |
| `FieldLabel` | `<label>` wrapper with optional `tooltipKey` and `after` props — for real form labels. |

### When To Use Each API

| Situation | Use |
|---|---|
| Form `<label>` element (input, select, checkbox) | `<FieldLabel tooltipKey="xxx">Label Text</FieldLabel>` |
| Form label with trailing badge/indicator | `<FieldLabel tooltipKey="xxx" after={<span>●</span>}>Label</FieldLabel>` |
| Table row label (`KeyValueTable`, `KVRow`) | `labelWithTip('Label Text', 'tooltipKey')` |
| Dynamic label in a loop where key is a variable | `<ParamTooltip tooltipKey={key as TooltipKey} />` |
| One-off tooltip not in the shared registry | `<ParamTooltip text="Custom explanation..." />` |

### Rules

1. **Always add new tooltip entries to `PARAM_TOOLTIPS`** when adding a new parameter or setting to any form/panel. Do not leave fields without explanations.

2. **Never use `string` for tooltip key types.** Use `TooltipKey` in component props, function parameters, and tuple types (e.g., `MetadataRowSpec` in `ModelCard.tsx`).

3. **Never use the old verbose pattern.** This is banned:
   ```tsx
   // ❌ DO NOT DO THIS
   {PARAM_TOOLTIPS.xxx && <ParamTooltip text={PARAM_TOOLTIPS.xxx} />}
   ```

4. **Prefer `FieldLabel` over bare `<label>` + inline tooltip.** When a component renders a `<label>` element with a tooltip, use `FieldLabel` instead of manually composing `<label>` + `<ParamTooltip>`.

5. **Keep tooltip text in `PARAM_TOOLTIPS` hardware-neutral and context-independent** so the same entry works across the VRAM calculator, model playground, catalog editor, and chat settings.

6. **Use `text` prop only for truly local, one-off explanations** that don't belong in the shared registry (e.g., composite explanations specific to one UI context).

### Adding a New Tooltipped Field

1. Add the explanation string to `PARAM_TOOLTIPS` in `ParamTooltips.tsx`.
2. Use `FieldLabel` (for form labels) or `labelWithTip` (for table rows) in your component.
3. TypeScript will enforce the key exists — a typo will be a compile error.

### Existing Pattern Reference

- **CatalogEditor.tsx** — `NullableNumInput` and `TriStateSelect` accept `tooltipKey?: TooltipKey`
- **ChatPanel.tsx** — `FieldLabel` with `after` prop for default-value dots
- **ModelPlayground.tsx** — `FieldLabel` with `htmlFor` for config controls
- **ConfigSweepParams.tsx / SamplingSweepParams.tsx** — `FieldLabel` with `className="playground-sweep-param-toggle"`
- **VRAMControls.tsx** — Mix of `FieldLabel` (static labels) and `ParamTooltip text="..."` (one-off)
- **VRAMResults.tsx** — `labelWithTip` for `KeyValueTable` rows
- **ModelCard.tsx** — `MetadataRowSpec` tuple typed as `[label, key, tooltipKey?: TooltipKey]`
