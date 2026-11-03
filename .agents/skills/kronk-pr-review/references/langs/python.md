# Python idioms

**Tools:** an LSP (pyright/pylsp) if available, else `rg`. **Checks:** `ruff`, `ruff-format`, `mypy` (when the repo configures it; for an untrusted change with default settings, since the config can load plugins), `bandit`, `pip-audit` (pins as written, nothing resolved; unpinned requirements are reported, not audited); `pytest` for a maintainer PR only. The repo's own tools (tox/nox/uv/poetry scripts) arrive as extra checks when approved, for a maintainer PR only.

## Correctness
Mutable default arguments; late-binding closures in loops; `is` vs `==`; shadowed builtins; naive vs aware `datetime`; blocking calls inside `async def`; un-awaited coroutines; resource handling without `with`; import-time side effects; type hints that contradict behavior.

## Failure paths
Bare `except:` / `except Exception: pass`; `raise` losing the cause (`raise ... from err`); broad `try` blocks; `assert` used for runtime validation (stripped with `-O`); return `None` on failure where callers expect a value.

## Contracts
Public functions/classes (`__all__`, no leading `_`) and their signatures; dataclass/pydantic models and their defaults/validators; API schemas; renamed keyword arguments break callers.

## Tests
pytest fixtures and parametrization; error paths with `pytest.raises(match=…)`; mocks patching the wrong target or hiding the real call; async tests; tests that depend on order or wall-clock time.

## Security
`subprocess` with `shell=True`, `eval`/`exec`, `pickle`/`yaml.load` on untrusted data, SQL string formatting, `requests` with `verify=False`, path traversal, `random` for tokens, secrets in settings/fixtures; dependency advisories (`pip-audit`).

## Clarity
Docstrings match signatures and raised exceptions; comprehensions only while readable.
