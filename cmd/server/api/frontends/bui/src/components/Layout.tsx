import { useState, type ReactNode } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { type Page, routeMap, pathToPage } from '../App';

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
    items: [{ page: 'catalog-list', label: 'List' }],
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
        id: 'docs-sdk',
        label: 'SDK',
        items: [
          { page: 'docs-sdk-kronk', label: 'Kronk' },
          { page: 'docs-sdk-model', label: 'Model' },
          { page: 'docs-sdk-examples', label: 'Examples' },
        ],
      },
      {
        id: 'docs-cli-sub',
        label: 'CLI',
        items: [
          { page: 'docs-cli-catalog', label: 'catalog' },
          { page: 'docs-cli-libs', label: 'libs' },
          { page: 'docs-cli-model', label: 'model' },
          { page: 'docs-cli-security', label: 'security' },
          { page: 'docs-cli-server', label: 'server' },
        ],
      },
      {
        id: 'docs-api-sub',
        label: 'Web API',
        items: [
          { page: 'docs-api-chat', label: 'Chat' },
          { page: 'docs-api-embeddings', label: 'Embeddings' },
          { page: 'docs-api-tools', label: 'Tools' },
        ],
      },
    ],
  },
];

export default function Layout({ children }: LayoutProps) {
  const location = useLocation();
  const currentPage = pathToPage[location.pathname] || 'home';
  const [expandedCategories, setExpandedCategories] = useState<Set<string>>(new Set());

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

  const renderMenuItem = (item: MenuItem) => (
    <Link
      key={item.page}
      to={routeMap[item.page]}
      className={`menu-item ${currentPage === item.page ? 'active' : ''}`}
    >
      {item.label}
    </Link>
  );

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
      </aside>
      <main className="main-content">{children}</main>
    </div>
  );
}
