import { useEffect, useRef, useState } from 'react';
import Prism from 'prismjs';
import 'prismjs/components/prism-go';
import 'prismjs/components/prism-python';
import 'prismjs/components/prism-javascript';
import 'prismjs/components/prism-typescript';
import 'prismjs/components/prism-rust';
import 'prismjs/components/prism-sql';
import 'prismjs/components/prism-json';
import 'prismjs/components/prism-yaml';
import 'prismjs/components/prism-bash';
import 'prismjs/themes/prism-tomorrow.css';

// Alias "shell" → "bash" so <code className="language-shell"> gets highlighted.
if (Prism.languages.bash && !Prism.languages.shell) {
  Prism.languages.shell = Prism.languages.bash;
}

interface CodeBlockProps {
  code: string;
  language?: string;
  collapsible?: boolean; // If true, collapse blocks > 3 lines. If false, show all content by default.
}

export default function CodeBlock({ code, language = 'go', collapsible = false }: CodeBlockProps) {
  const codeRef = useRef<HTMLElement>(null);
  const [isExpanded, setIsExpanded] = useState(false);
  const [isCopied, setIsCopied] = useState(false);
  const [lineCount, setLineCount] = useState(0);

  useEffect(() => {
    if (codeRef.current) {
      Prism.highlightElement(codeRef.current);
      const lines = code.split('\n').length;
      setLineCount(lines);
      console.log('CodeBlock: Lines:', lines, 'Collapse?:', lines > 3);
    }
  }, [code]);

  const handleCopy = async () => {
    try {
      console.log('Attempting to copy code:', { codeLength: code.length, code: code.substring(0, 100) });
      
      // Use the modern Clipboard API
      if (navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(code);
        console.log('Successfully copied to clipboard');
        setIsCopied(true);
        setTimeout(() => setIsCopied(false), 2000);
      } else {
        // Fallback for older browsers
        const textArea = document.createElement('textarea');
        textArea.value = code;
        textArea.style.position = 'fixed';
        textArea.style.left = '-999999px';
        document.body.appendChild(textArea);
        textArea.focus();
        textArea.select();
        const success = document.execCommand('copy');
        document.body.removeChild(textArea);
        
        if (success) {
          console.log('Successfully copied to clipboard (fallback)');
          setIsCopied(true);
          setTimeout(() => setIsCopied(false), 2000);
        } else {
          console.error('Fallback copy failed');
        }
      }
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  };

  const shouldCollapse = collapsible && lineCount > 3;
  const displayClassName = language === 'go' ? 'code-go' : 'code-block';
  const languageClass = `language-${language}`;
  const preClassName = `${displayClassName} ${shouldCollapse && !isExpanded ? 'collapsed' : ''}`;

  console.log('CodeBlock render:', { lineCount, shouldCollapse, isExpanded, preClassName });

  return (
    <div className="code-block-container">
      <div className="code-block-header">
        <div className="code-block-info">
          {shouldCollapse && (
            <button
              className="code-block-toggle"
              onClick={() => setIsExpanded(!isExpanded)}
              aria-label={isExpanded ? 'Collapse code' : 'Expand code'}
              title={isExpanded ? 'Collapse' : 'Expand'}
            >
              <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
                {isExpanded ? (
                  <path d="M2 5l6 6 6-6H2z" />
                ) : (
                  <path d="M6 2v12l6-6-6-6z" />
                )}
              </svg>
              <span className="code-block-line-count">
                {isExpanded ? 'Collapse' : `Show all (${lineCount} lines)`}
              </span>
            </button>
          )}
        </div>
        <button
          className="code-copy-btn"
          onClick={handleCopy}
          aria-label="Copy code"
          title="Copy to clipboard"
        >
          <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
            <path d="M4 1.5H3a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h9a2 2 0 0 0 2-2V3.5a2 2 0 0 0-2-2h-1v-1a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v1zm5-1v1H6v-1h3z"/>
            <path d="M4 5.5a.5.5 0 0 1 .5-.5h7a.5.5 0 0 1 .5.5v9a.5.5 0 0 1-.5.5H4.5a.5.5 0 0 1-.5-.5v-9z"/>
          </svg>
          <span className={`copy-feedback ${isCopied ? 'show' : ''}`}>
            {isCopied ? 'Copied!' : 'Copy'}
          </span>
        </button>
      </div>
      <pre className={preClassName}>
        <code
          ref={codeRef}
          className={languageClass}
        >
          {code}
        </code>
      </pre>
    </div>
  );
}
