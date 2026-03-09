import { useState, useRef, useCallback, useMemo, useEffect } from 'react';
import type { KeyboardEvent, MouseEvent } from 'react';

export interface ModelSelectorExtraItem {
  id: string;
  label: string;
}

interface ModelSelectorProps {
  models: { id: string; draft_model_id?: string }[] | undefined;
  selectedModel: string | null;
  onSelect: (id: string) => void;
  disabled?: boolean;
  placeholder?: string;
  extraItems?: ModelSelectorExtraItem[];
  draftModelIds?: Set<string>;
}

// ── Grouping types ──

type StandaloneNode = {
  type: 'standalone';
  id: string;
};

type GroupNode = {
  type: 'group';
  base: string;
  baseId: string | null; // non-null when the base itself is a real model
  variants: { id: string; suffix: string }[];
};

type RenderNode = StandaloneNode | GroupNode;

function buildGroups(models: { id: string }[]): RenderNode[] {
  const groupMap = new Map<string, { baseId: string | null; variants: { id: string; suffix: string }[]; order: number }>();
  let orderCounter = 0;

  for (const m of models) {
    const lastSlash = m.id.lastIndexOf('/');
    if (lastSlash === -1) {
      const existing = groupMap.get(m.id);
      if (existing) {
        existing.baseId = m.id;
      } else {
        groupMap.set(m.id, { baseId: m.id, variants: [], order: orderCounter++ });
      }
    } else {
      const base = m.id.slice(0, lastSlash);
      const suffix = m.id.slice(lastSlash + 1);
      if (!base || !suffix) continue;
      const existing = groupMap.get(base);
      if (existing) {
        existing.variants.push({ id: m.id, suffix });
      } else {
        groupMap.set(base, { baseId: null, variants: [{ id: m.id, suffix }], order: orderCounter++ });
      }
    }
  }

  const sorted = Array.from(groupMap.entries()).sort((a, b) => a[1].order - b[1].order);

  const nodes: RenderNode[] = [];
  for (const [base, group] of sorted) {
    if (group.variants.length === 0 && group.baseId) {
      nodes.push({ type: 'standalone', id: group.baseId });
    } else {
      nodes.push({
        type: 'group',
        base,
        baseId: group.baseId,
        variants: group.variants,
      });
    }
  }

  return nodes;
}

function isInGroup(node: GroupNode, modelId: string | null): boolean {
  if (!modelId) return false;
  if (node.baseId === modelId) return true;
  return node.variants.some((v) => v.id === modelId);
}

// ── Helpers ──

function getVisibleItems(container: HTMLElement): HTMLElement[] {
  return Array.from(container.querySelectorAll<HTMLElement>('[role="treeitem"]:not([aria-disabled="true"])'));
}

function Chevron({ expanded }: { expanded: boolean }) {
  return (
    <span
      className={`model-selector-chevron${expanded ? ' expanded' : ''}`}
      data-role="toggle"
      aria-hidden="true"
    >
      ▶
    </span>
  );
}

// ── Component ──

export default function ModelSelector({
  models,
  selectedModel,
  onSelect,
  disabled = false,
  placeholder = 'Select a model...',
  extraItems,
  draftModelIds: externalDraftIds,
}: ModelSelectorProps) {
  const listRef = useRef<HTMLDivElement>(null);
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());

  const items = models ?? [];
  const nodes = useMemo(() => buildGroups(items), [items]);

  const draftModelIds = useMemo(() => {
    if (externalDraftIds) return externalDraftIds;
    const ids = new Set<string>();
    for (const m of items) {
      if (m.draft_model_id) ids.add(m.id);
    }
    return ids;
  }, [items, externalDraftIds]);
  const hasItems = nodes.length > 0 || (extraItems && extraItems.length > 0);

  // Auto-expand the group containing the selected model.
  useEffect(() => {
    if (!selectedModel) return;
    for (const node of nodes) {
      if (node.type === 'group' && isInGroup(node, selectedModel)) {
        setExpanded((prev) => {
          if (prev.has(node.base)) return prev;
          const next = new Set(prev);
          next.add(node.base);
          return next;
        });
        break;
      }
    }
  }, [selectedModel, nodes]);

  const toggleGroup = useCallback((base: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(base)) {
        next.delete(base);
      } else {
        next.add(base);
      }
      return next;
    });
  }, []);

  const moveFocus = useCallback((from: HTMLElement, direction: 'up' | 'down') => {
    const container = listRef.current;
    if (!container) return;
    const visible = getVisibleItems(container);
    const idx = visible.indexOf(from);
    const next = direction === 'down' ? idx + 1 : idx - 1;
    if (next >= 0 && next < visible.length) {
      visible[next].focus();
      visible[next].scrollIntoView({ block: 'nearest' });
    }
  }, []);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLElement>) => {
      if (disabled) return;

      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault();
        moveFocus(e.currentTarget, e.key === 'ArrowDown' ? 'down' : 'up');
        return;
      }

      const base = e.currentTarget.dataset.groupBase;
      if (base) {
        if (e.key === 'ArrowRight') {
          e.preventDefault();
          setExpanded((prev) => {
            if (prev.has(base)) {
              // Already expanded: move focus to first child.
              requestAnimationFrame(() => {
                const container = listRef.current;
                if (!container) return;
                const group = container.querySelector<HTMLElement>(`[data-group-container="${base}"]`);
                const firstChild = group?.querySelector<HTMLElement>('[role="treeitem"]');
                if (firstChild) {
                  firstChild.focus();
                  firstChild.scrollIntoView({ block: 'nearest' });
                }
              });
              return prev;
            }
            const next = new Set(prev);
            next.add(base);
            return next;
          });
          return;
        }
        if (e.key === 'ArrowLeft') {
          e.preventDefault();
          setExpanded((prev) => {
            if (!prev.has(base)) return prev;
            const next = new Set(prev);
            next.delete(base);
            return next;
          });
          return;
        }
      }

      // Left on a child item: move focus to parent group header.
      if (!base && e.key === 'ArrowLeft') {
        e.preventDefault();
        const groupContainer = e.currentTarget.closest<HTMLElement>('[data-group-container]');
        if (groupContainer) {
          const header = groupContainer.previousElementSibling as HTMLElement | null;
          if (header && header.getAttribute('role') === 'treeitem') {
            header.focus();
            header.scrollIntoView({ block: 'nearest' });
          }
        }
        return;
      }

      // Enter/Space on items that aren't natively buttons.
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        e.currentTarget.click();
      }
    },
    [disabled, moveFocus],
  );

  const handleHeaderClick = useCallback(
    (e: MouseEvent<HTMLElement>, node: GroupNode) => {
      if (disabled) return;
      const target = e.target as HTMLElement;
      if (target.closest('[data-role="toggle"]')) {
        toggleGroup(node.base);
      } else if (node.baseId) {
        onSelect(node.baseId);
      } else {
        toggleGroup(node.base);
      }
    },
    [disabled, onSelect, toggleGroup],
  );

  return (
    <div
      className={`model-selector${disabled ? ' model-selector-disabled' : ''}`}
      role="tree"
      aria-label={placeholder}
      aria-multiselectable={false}
      ref={listRef}
    >
      {!hasItems && (
        <div className="model-selector-empty">{placeholder}</div>
      )}

      {nodes.map((node) => {
        if (node.type === 'standalone') {
          return (
            <div
              key={node.id}
              role="treeitem"
              aria-selected={selectedModel === node.id}
              aria-level={1}
              aria-disabled={disabled}
              tabIndex={disabled ? -1 : 0}
              className={`model-selector-item${selectedModel === node.id ? ' active' : ''}`}
              onClick={() => { if (!disabled) onSelect(node.id); }}
              onKeyDown={handleKeyDown}
              title={node.id}
            >
              <span className="model-selector-label">{draftModelIds.has(node.id) ? '⚡ ' : ''}{node.id}</span>
            </div>
          );
        }

        const isExpanded = expanded.has(node.base);
        const groupActive = isInGroup(node, selectedModel);
        const groupHasDraft = node.baseId ? draftModelIds.has(node.baseId) : node.variants.some((v) => draftModelIds.has(v.id));

        return (
          <div key={node.base} className="model-selector-group">
            <div
              role="treeitem"
              aria-expanded={isExpanded}
              aria-selected={node.baseId ? selectedModel === node.baseId : false}
              aria-level={1}
              aria-disabled={disabled}
              data-group-base={node.base}
              tabIndex={disabled ? -1 : 0}
              className={`model-selector-item model-selector-group-header${groupActive ? ' active' : ''}`}
              onClick={(e) => handleHeaderClick(e, node)}
              onKeyDown={handleKeyDown}
              title={node.base}
            >
              <Chevron expanded={isExpanded} />
              <span className="model-selector-label">{groupHasDraft ? '⚡ ' : ''}{node.base}</span>
            </div>
            {isExpanded && (
              <div role="group" data-group-container={node.base}>
                {node.variants.map((v) => (
                  <div
                    key={v.id}
                    role="treeitem"
                    aria-selected={selectedModel === v.id}
                    aria-level={2}
                    aria-disabled={disabled}
                    tabIndex={disabled ? -1 : 0}
                    className={`model-selector-item model-selector-subitem${selectedModel === v.id ? ' active' : ''}`}
                    onClick={() => { if (!disabled) onSelect(v.id); }}
                    onKeyDown={handleKeyDown}
                    title={v.id}
                  >
                    <span className="model-selector-label">{draftModelIds.has(v.id) ? '⚡ ' : ''}{v.suffix}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        );
      })}

      {extraItems?.map((item) => (
        <div
          key={item.id}
          role="treeitem"
          aria-selected={selectedModel === item.id}
          aria-level={1}
          aria-disabled={disabled}
          tabIndex={disabled ? -1 : 0}
          className={`model-selector-item model-selector-extra${selectedModel === item.id ? ' active' : ''}`}
          onClick={() => { if (!disabled) onSelect(item.id); }}
          onKeyDown={handleKeyDown}
          title={item.label}
        >
          <span className="model-selector-label">{item.label}</span>
        </div>
      ))}
    </div>
  );
}
