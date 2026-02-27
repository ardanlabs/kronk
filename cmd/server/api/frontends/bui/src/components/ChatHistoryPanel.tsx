import { useState, useMemo, useCallback } from 'react';
import { useChatHistory, type SavedChat, type HistoryMessage } from '../contexts/ChatHistoryContext';
import ConfirmDialog from './ConfirmDialog';

interface ChatHistoryPanelProps {
  isOpen: boolean;
  onClose: () => void;
  onLoadChat: (messages: HistoryMessage[]) => void;
}

function formatRelativeDate(timestamp: number): string {
  const now = Date.now();
  const diffMs = now - timestamp;
  const diffSec = Math.floor(diffMs / 1000);
  const diffMin = Math.floor(diffSec / 60);
  const diffHr = Math.floor(diffMin / 60);
  const diffDays = Math.floor(diffHr / 24);
  const diffMonths = Math.floor(diffDays / 30);

  if (diffSec < 60) return 'just now';
  if (diffMin < 60) return `${diffMin} min ago`;
  if (diffHr < 24) return `${diffHr} hr ago`;
  if (diffDays < 30) return `${diffDays} days ago`;
  return `${diffMonths} months ago`;
}

function formatChatForClipboard(messages: HistoryMessage[]): string {
  return messages
    .map((msg) => {
      const role = msg.role === 'user' ? 'USER' : 'MODEL';
      let text = '';

      if (msg.attachments && msg.attachments.length > 0) {
        const names = msg.attachments.map((a) => a.name).join(', ');
        text += `[Attached: ${names}]\n`;
      }

      text += msg.content;
      return `${role}:\n${text}`;
    })
    .join('\n\n');
}

export default function ChatHistoryPanel({ isOpen, onClose, onLoadChat }: ChatHistoryPanelProps) {
  const { history, deleteChats } = useChatHistory();
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false);

  // History is already sorted newest first from the context.
  const sortedChats = useMemo(
    () => [...history],
    [history],
  );

  const toggleSelection = useCallback((id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }, []);

  const allSelected = sortedChats.length > 0 && selectedIds.size === sortedChats.length;

  const toggleSelectAll = useCallback(() => {
    if (allSelected) {
      setSelectedIds(new Set());
    } else {
      setSelectedIds(new Set(sortedChats.map((c) => c.id)));
    }
  }, [allSelected, sortedChats]);

  const handleDeleteSelected = useCallback(() => {
    if (selectedIds.size === 0) return;
    setConfirmDeleteOpen(true);
  }, [selectedIds]);

  const handleConfirmDelete = useCallback(() => {
    deleteChats(Array.from(selectedIds));
    setSelectedIds(new Set());
    setConfirmDeleteOpen(false);
  }, [selectedIds, deleteChats]);

  const handleLoadChat = useCallback(
    (chat: SavedChat) => {
      onLoadChat(chat.messages);
      onClose();
    },
    [onLoadChat, onClose],
  );

  const handleCopy = useCallback(
    (e: React.MouseEvent, chat: SavedChat) => {
      e.stopPropagation();
      const text = formatChatForClipboard(chat.messages);
      navigator.clipboard.writeText(text);
    },
    [],
  );

  const panelClassName = `chat-history-panel${isOpen ? ' chat-history-panel-open' : ''}`;

  return (
    <div className={panelClassName}>
      <div className="chat-history-header">
        <span>Chat History</span>
        <button onClick={onClose} aria-label="Close chat history">×</button>
      </div>

      {sortedChats.length > 0 && (
        <div className="chat-history-toolbar">
          <label className="chat-history-select-all">
            <input
              type="checkbox"
              checked={allSelected}
              onChange={toggleSelectAll}
            />
            Select All
          </label>

          {selectedIds.size > 0 && (
            <>
              <span>{selectedIds.size} selected</span>
              <button
                className="chat-history-delete-btn"
                onClick={handleDeleteSelected}
              >
                Delete
              </button>
            </>
          )}
        </div>
      )}

      {sortedChats.length === 0 ? (
        <div className="chat-history-empty">No saved chats</div>
      ) : (
        <div className="chat-history-list">
          {sortedChats.map((chat) => {
            const isSelected = selectedIds.has(chat.id);
            const itemClass = `chat-history-item${isSelected ? ' chat-history-item-selected' : ''}`;

            return (
              <div
                key={chat.id}
                className={itemClass}
                onClick={() => handleLoadChat(chat)}
              >
                <input
                  type="checkbox"
                  className="chat-history-item-checkbox"
                  checked={isSelected}
                  onChange={() => toggleSelection(chat.id)}
                  onClick={(e) => e.stopPropagation()}
                />

                <div className="chat-history-item-info">
                  <div className="chat-history-item-title">{chat.title}</div>
                  <div className="chat-history-item-model">{chat.model}</div>
                  <div className="chat-history-item-date">
                    {formatRelativeDate(chat.savedAt)}
                  </div>
                </div>

                <button
                  className="chat-history-item-copy"
                  onClick={(e) => handleCopy(e, chat)}
                  title="Copy chat to clipboard"
                >
                  Copy
                </button>
              </div>
            );
          })}
        </div>
      )}

      <ConfirmDialog
        isOpen={confirmDeleteOpen}
        message={`Delete ${selectedIds.size} selected chat(s)?`}
        confirmLabel="Delete"
        cancelLabel="Cancel"
        onConfirm={handleConfirmDelete}
        onCancel={() => setConfirmDeleteOpen(false)}
      />
    </div>
  );
}
