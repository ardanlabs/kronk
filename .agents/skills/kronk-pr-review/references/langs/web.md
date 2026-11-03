# Vue / JS / TS / CSS idioms

Kronk's BUI is React/TS (`cmd/server/api/frontends/bui`); follow its `src/components/AGENTS.md`.

**Tools:** an LSP if available, else `rg`/`ast-grep`. **Checks:** `bui-tsc` (`tsc --noEmit`, maintainer PR only; a type error in changed BUI code is G4: the BUI build and server embed fail), `jscpd` (clones); the repo's own `lint`/`typecheck`/`test` scripts arrive as extra checks when approved, for a maintainer PR only.

## Correctness
Hook dependencies, effect cleanup (timers, streams, `AbortController`), reactivity (mutating props, lost reactivity on destructuring, `watch` without cleanup), leaked listeners/timers/subscriptions, unhandled async, race between overlapping requests, stale closures, `==` coercion, a11y (labels, roles, keyboard, focus).

## Failure paths
Promises and `await` caught; non-2xx and SSE error events handled; `catch` doesn't swallow; the UI shows failure instead of empty or stale state.

## Contracts
`props`/`emits`: required vs optional, types, defaults, payload shapes; props immutable; internal state not leaked; exported types and API client shapes (`src/types`) match the Go JSON tags exactly (spelling, optionality, numeric vs string enums).

## Tests
The BUI has no test runner: `bui-tsc` is the automated check; name the manual check (page, action, expected result).

## Security
XSS (`v-html`, `innerHTML`, `dangerouslySetInnerHTML`), secrets or tokens in bundles/localStorage, open redirects, `postMessage` origin checks, new or unpinned dependencies.

## Clarity
JSDoc and component comments match the real props and behavior; tooltips go through `PARAM_TOOLTIPS`/`TooltipKey` and match the parameter's real behavior. `static/assets` and the generated `Docs*.tsx` components are rebuilt with their sources.
