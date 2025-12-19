import { useState, type ReactNode } from 'react';
import type { Page } from '../App';

interface LayoutProps {
  children: ReactNode;
  currentPage: Page;
  onNavigate: (page: Page) => void;
}

interface MenuCategory {
  id: string;
  label: string;
  items?: MenuItem[];
  subcategories?: MenuCategory[];
}

interface MenuItem {
  id: Page;
  label: string;
}

const menuStructure: MenuCategory[] = [
  {
    id: 'model',
    label: 'Model',
    items: [
      { id: 'model-list', label: 'List' },
      { id: 'model-ps', label: 'Running' },
      { id: 'model-pull', label: 'Pull' },
      { id: 'model-remove', label: 'Remove' },
    ],
  },
  {
    id: 'catalog',
    label: 'Catalog',
    items: [
      { id: 'catalog-list', label: 'List' },
      { id: 'catalog-pull', label: 'Pull' },
    ],
  },
  {
    id: 'libs',
    label: 'Libs',
    items: [{ id: 'libs-pull', label: 'Pull' }],
  },
  {
    id: 'security',
    label: 'Security',
    subcategories: [
      {
        id: 'security-key',
        label: 'Key',
        items: [
          { id: 'security-key-list', label: 'List' },
          { id: 'security-key-create', label: 'Create' },
          { id: 'security-key-delete', label: 'Delete' },
        ],
      },
      {
        id: 'security-token',
        label: 'Token',
        items: [{ id: 'security-token-create', label: 'Create' }],
      },
    ],
  },
];

export default function Layout({ children, currentPage, onNavigate }: LayoutProps) {
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
      return category.items.some((item) => item.id === currentPage);
    }
    if (category.subcategories) {
      return category.subcategories.some((sub) => isCategoryActive(sub));
    }
    return false;
  };

  const renderMenuItem = (item: MenuItem) => (
    <div
      key={item.id}
      className={`menu-item ${currentPage === item.id ? 'active' : ''}`}
      onClick={() => onNavigate(item.id)}
    >
      {item.label}
    </div>
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
          {category.items?.map(renderMenuItem)}
          {category.subcategories?.map((sub) => renderCategory(sub, true))}
        </div>
      </div>
    );
  };

  return (
    <div className="app">
      <aside className="sidebar">
        <div className="sidebar-header">
          <h1>Kronk</h1>
        </div>
        <nav>{menuStructure.map((category) => renderCategory(category))}</nav>
      </aside>
      <main className="main-content">{children}</main>
    </div>
  );
}
