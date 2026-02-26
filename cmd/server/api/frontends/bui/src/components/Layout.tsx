import { useState, useEffect, type ReactNode } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { type Page, routeMap, pathToPage } from '../App';
import { useDownload } from '../contexts/DownloadContext';
import { useAutoTestRunner } from '../contexts/AutoTestRunnerContext';

interface LayoutProps {
  children: ReactNode;
}

interface MenuCategory {
  id: string;
  label: string;
  items?: MenuItem[];
  subcategories?: MenuCategory[];
}

interface MenuItem {
  page: Page;
  label: string;
  hash?: string;
}

const menuStructure: MenuCategory[] = [
  {
    id: 'settings',
    label: 'Settings',
    items: [{ page: 'settings', label: 'API Token' }],
  },
  {
    id: 'model',
    label: 'Models',
    items: [
      { page: 'model-list', label: 'List' },
      { page: 'model-ps', label: 'Running' },
      { page: 'model-pull', label: 'Pull' },
    ],
  },
  {
    id: 'catalog',
    label: 'Catalog',
    items: [
      { page: 'catalog-list', label: 'List' },
      { page: 'catalog-editor', label: 'Editor' },
    ],
  },
  {
    id: 'libs',
    label: 'Libs',
    items: [{ page: 'libs-pull', label: 'Pull' }],
  },
  {
    id: 'security',
    label: 'Security',
    subcategories: [
      {
        id: 'security-key',
        label: 'Key',
        items: [
          { page: 'security-key-list', label: 'List' },
          { page: 'security-key-create', label: 'Create' },
          { page: 'security-key-delete', label: 'Delete' },
        ],
      },
      {
        id: 'security-token',
        label: 'Token',
        items: [{ page: 'security-token-create', label: 'Create' }],
      },
    ],
  },
  {
    id: 'docs',
    label: 'Docs',
    subcategories: [
      {
        id: 'docs-manual-sub',
        label: 'Manual',
        items: [
          { page: 'docs-manual', label: 'Introduction', hash: 'chapter-1-introduction' },
          { page: 'docs-manual', label: 'Installation & Quick Start', hash: 'chapter-2-installation-quick-start' },
          { page: 'docs-manual', label: 'Model Configuration', hash: 'chapter-3-model-configuration' },
          { page: 'docs-manual', label: 'Batch Processing', hash: 'chapter-4-batch-processing' },
          { page: 'docs-manual', label: 'Message Caching', hash: 'chapter-5-message-caching' },
          { page: 'docs-manual', label: 'YaRN Extended Context', hash: 'chapter-6-yarn-extended-context' },
          { page: 'docs-manual', label: 'Model Server', hash: 'chapter-7-model-server' },
          { page: 'docs-manual', label: 'API Endpoints', hash: 'chapter-8-api-endpoints' },
          { page: 'docs-manual', label: 'Request Parameters', hash: 'chapter-9-request-parameters' },
          { page: 'docs-manual', label: 'Multi-Modal Models', hash: 'chapter-10-multi-modal-models' },
          { page: 'docs-manual', label: 'Security & Authentication', hash: 'chapter-11-security-authentication' },
          { page: 'docs-manual', label: 'Browser UI (BUI)', hash: 'chapter-12-browser-ui-bui' },
          { page: 'docs-manual', label: 'Client Integration', hash: 'chapter-13-client-integration' },
          { page: 'docs-manual', label: 'Observability', hash: 'chapter-14-observability' },
          { page: 'docs-manual', label: 'MCP Service', hash: 'chapter-15-mcp-service' },
          { page: 'docs-manual', label: 'Troubleshooting', hash: 'chapter-16-troubleshooting' },
          { page: 'docs-manual', label: 'Developer Guide', hash: 'chapter-17-developer-guide' },
        ],
      },
      {
        id: 'docs-sdk',
        label: 'SDK',
        items: [
          { page: 'docs-sdk-kronk', label: 'Kronk' },
          { page: 'docs-sdk-model', label: 'Model' },
          { page: 'docs-sdk-examples', label: 'Examples' },
          { page: 'docs-sdk-examples', label: 'Audio', hash: 'example-audio' },
          { page: 'docs-sdk-examples', label: 'Chat', hash: 'example-chat' },
          { page: 'docs-sdk-examples', label: 'Embedding', hash: 'example-embedding' },
          { page: 'docs-sdk-examples', label: 'Grammar', hash: 'example-grammar' },
          { page: 'docs-sdk-examples', label: 'Question', hash: 'example-question' },
          { page: 'docs-sdk-examples', label: 'Rerank', hash: 'example-rerank' },
          { page: 'docs-sdk-examples', label: 'Response', hash: 'example-response' },
          { page: 'docs-sdk-examples', label: 'Vision', hash: 'example-vision' },
        ],
      },
      {
        id: 'docs-cli-sub',
        label: 'CLI',
        items: [
          { page: 'docs-cli-catalog', label: 'catalog' },
          { page: 'docs-cli-libs', label: 'libs' },
          { page: 'docs-cli-model', label: 'model' },
          { page: 'docs-cli-run', label: 'run' },
          { page: 'docs-cli-security', label: 'security' },
          { page: 'docs-cli-server', label: 'server' },
        ],
      },
      {
        id: 'docs-api-sub',
        label: 'Web API',
        items: [
          { page: 'docs-api-chat', label: 'Chat' },
          { page: 'docs-api-messages', label: 'Messages' },
          { page: 'docs-api-responses', label: 'Responses' },
          { page: 'docs-api-embeddings', label: 'Embeddings' },
          { page: 'docs-api-rerank', label: 'Rerank' },
          { page: 'docs-api-tokenize', label: 'Tokenize' },
          { page: 'docs-api-tools', label: 'Tools' },
        ],
      },
    ],
  },
  {
    id: 'apps',
    label: 'Apps',
    items: [
      { page: 'chat', label: 'Chat' },
      { page: 'playground', label: 'Playground' },
      { page: 'vram-calculator', label: 'VRAM Calculator' },
    ],
  },
];

export default function Layout({ children }: LayoutProps) {
  const location = useLocation();
  const navigate = useNavigate();
  const currentPage = pathToPage[location.pathname] || 'home';
  const [expandedCategories, setExpandedCategories] = useState<Set<string>>(new Set());
  const { download, isDownloading } = useDownload();
  const { run, isRunning: isAutoTesting, stopRun } = useAutoTestRunner();

  const autoTestTitle = (() => {
    if (!run) return '';
    if (isAutoTesting) return 'Testing...';
    switch (run.status) {
      case 'completed': return 'Completed';
      case 'cancelled': return 'Cancelled';
      case 'error': return 'Failed';
      default: return 'Testing';
    }
  })();

  const autoTestSubtitle = (() => {
    if (!run) return undefined;
    if (run.status === 'error' && run.errorMessage) return run.errorMessage;
    if (isAutoTesting && run.totalTrials === 0) {
      return run.calibrationStatus ?? run.templateRepairStatus ?? 'Preparing...';
    }
    if (run.totalTrials > 0) {
      return `Trial ${Math.min(run.currentTrialIndex + (isAutoTesting ? 1 : 0), run.totalTrials)}/${run.totalTrials}`;
    }
    return undefined;
  })();

  const autoTestLogLine = (() => {
    if (!run || !isAutoTesting) return undefined;
    const runningTrial = run.trials.find(t => t?.status === 'running');
    const entries = runningTrial?.logEntries;
    if (entries && entries.length > 0) return entries[entries.length - 1].message;
    return undefined;
  })();

  const showAutoTestIndicator = !!run;
  const showDownloadIndicator = !!download;

  // Auto-expand categories that contain the current page
  useEffect(() => {
    const findCategoryPath = (categories: MenuCategory[], targetPage: Page): string[] => {
      for (const category of categories) {
        if (category.items?.some((item) => item.page === targetPage)) {
          return [category.id];
        }
        if (category.subcategories) {
          const subPath = findCategoryPath(category.subcategories, targetPage);
          if (subPath.length > 0) {
            return [category.id, ...subPath];
          }
        }
      }
      return [];
    };

    const categoryPath = findCategoryPath(menuStructure, currentPage);
    if (categoryPath.length > 0) {
      setExpandedCategories((prev) => {
        const next = new Set(prev);
        categoryPath.forEach((id) => next.add(id));
        return next;
      });
    }
  }, [currentPage]);

  const toggleCategory = (id: string) => {
    setExpandedCategories((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  const isCategoryActive = (category: MenuCategory): boolean => {
    if (category.items) {
      return category.items.some((item) => item.page === currentPage);
    }
    if (category.subcategories) {
      return category.subcategories.some((sub) => isCategoryActive(sub));
    }
    return false;
  };

  const renderMenuItem = (item: MenuItem) => {
    const path = routeMap[item.page];
    const isActive = currentPage === item.page && !item.hash;
    
    if (item.hash) {
      const handleClick = (e: React.MouseEvent) => {
        e.preventDefault();
        // Use React Router navigation to preserve state
        navigate(`${path}#${item.hash}`);
        // Scroll to the element after navigation
        setTimeout(() => {
          const element = document.getElementById(item.hash!);
          if (element) {
            element.scrollIntoView({ behavior: 'smooth' });
          }
        }, 100);
      };
      
      return (
        <a
          key={`${item.page}-${item.hash}`}
          href={`${path}#${item.hash}`}
          onClick={handleClick}
          className="menu-item"
        >
          {item.label}
        </a>
      );
    }
    
    return (
      <Link
        key={item.page}
        to={path}
        className={`menu-item ${isActive ? 'active' : ''}`}
      >
        {item.label}
      </Link>
    );
  };

  const renderCategory = (category: MenuCategory, isSubmenu = false) => {
    const isExpanded = expandedCategories.has(category.id);
    const isActive = isCategoryActive(category);

    return (
      <div key={category.id} className={`menu-category ${isSubmenu ? 'submenu' : ''}`}>
        <div
          className={`menu-category-header ${isActive ? 'active' : ''}`}
          onClick={() => toggleCategory(category.id)}
        >
          <span>{category.label}</span>
          <span className={`menu-category-arrow ${isExpanded ? 'expanded' : ''}`}>▶</span>
        </div>
        <div className={`menu-items ${isExpanded ? 'expanded' : ''}`}>
          {category.subcategories?.map((sub) => renderCategory(sub, true))}
          {category.items?.map(renderMenuItem)}
        </div>
      </div>
    );
  };

  return (
    <div className="app">
      <aside className="sidebar">
        <div className="sidebar-header">
          <Link to="/" style={{ textDecoration: 'none', color: 'inherit' }} className="sidebar-brand">
            <img src="/kronk-logo.png" alt="Kronk Logo" className="sidebar-logo" />
            <h1>Model Server</h1>
          </Link>
        </div>
        <nav>{menuStructure.map((category) => renderCategory(category))}</nav>
        {(showAutoTestIndicator || showDownloadIndicator) && (
          <div className="sidebar-indicators">
            {showAutoTestIndicator && (
              <div className="download-indicator">
                <div className="download-indicator-link autotest-indicator-link">
                  <Link to={routeMap['playground']} className="autotest-indicator-top">
                    <div className="download-indicator-header">
                      {isAutoTesting ? (
                        <span className="download-indicator-spinner" />
                      ) : run.status === 'completed' ? (
                        <span className="download-indicator-icon success">✓</span>
                      ) : (
                        <span className="download-indicator-icon error">✗</span>
                      )}
                      <span className="download-indicator-title">{autoTestTitle}</span>
                    </div>
                    {autoTestSubtitle && (
                      <div className="download-indicator-url" title={autoTestSubtitle} aria-live="polite">
                        {autoTestSubtitle}
                      </div>
                    )}
                    {autoTestLogLine && (
                      <div className="autotest-indicator-log" title={autoTestLogLine}>
                        {autoTestLogLine}
                      </div>
                    )}
                  </Link>
                  {isAutoTesting && (
                    <button type="button" className="autotest-indicator-stop" onClick={stopRun} aria-label="Stop automated testing" title="Stop automated testing">
                      ■
                    </button>
                  )}
                </div>
              </div>
            )}
            {showDownloadIndicator && (
              <div className="download-indicator">
                <Link to={routeMap['model-pull']} className="download-indicator-link">
                  <div className="download-indicator-header">
                    {isDownloading ? (
                      <span className="download-indicator-spinner" />
                    ) : download.status === 'complete' ? (
                      <span className="download-indicator-icon success">✓</span>
                    ) : (
                      <span className="download-indicator-icon error">✗</span>
                    )}
                    <span className="download-indicator-title">
                      {isDownloading ? 'Downloading...' : download.status === 'complete' ? 'Complete' : 'Failed'}
                    </span>
                  </div>
                  <div className="download-indicator-url" title={download.modelUrl}>
                    {download.modelUrl.split('/').pop()}
                  </div>
                </Link>
              </div>
            )}
          </div>
        )}
      </aside>
      <main className="main-content">{children}</main>
    </div>
  );
}
