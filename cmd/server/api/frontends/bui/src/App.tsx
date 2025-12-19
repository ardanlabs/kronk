import { useState } from 'react';
import Layout from './components/Layout';
import ModelList from './components/ModelList';
import ModelPs from './components/ModelPs';

import ModelPull from './components/ModelPull';
import ModelRemove from './components/ModelRemove';
import CatalogList from './components/CatalogList';

import CatalogPull from './components/CatalogPull';
import LibsPull from './components/LibsPull';
import SecurityKeyList from './components/SecurityKeyList';
import SecurityKeyCreate from './components/SecurityKeyCreate';
import SecurityKeyDelete from './components/SecurityKeyDelete';
import SecurityTokenCreate from './components/SecurityTokenCreate';
import { ModelListProvider } from './contexts/ModelListContext';

export type Page =
  | 'home'
  | 'model-list'
  | 'model-ps'
  | 'model-pull'
  | 'model-remove'
  | 'catalog-list'
  | 'catalog-pull'
  | 'libs-pull'
  | 'security-key-list'
  | 'security-key-create'
  | 'security-key-delete'
  | 'security-token-create';

function App() {
  const [currentPage, setCurrentPage] = useState<Page>('home');

  const renderPage = () => {
    switch (currentPage) {
      case 'model-list':
        return <ModelList />;
      case 'model-ps':
        return <ModelPs />;
      case 'model-pull':
        return <ModelPull />;
      case 'model-remove':
        return <ModelRemove />;
      case 'catalog-list':
        return <CatalogList />;
      case 'catalog-pull':
        return <CatalogPull />;
      case 'libs-pull':
        return <LibsPull />;
      case 'security-key-list':
        return <SecurityKeyList />;
      case 'security-key-create':
        return <SecurityKeyCreate />;
      case 'security-key-delete':
        return <SecurityKeyDelete />;
      case 'security-token-create':
        return <SecurityTokenCreate />;
      default:
        return (
          <div className="welcome">
            <div className="logo">◆</div>
            <h2>Welcome to Kronk</h2>
            <p>Select an option from the sidebar to manage your models, catalog, and security settings.</p>
          </div>
        );
    }
  };

  return (
    <ModelListProvider>
      <Layout currentPage={currentPage} onNavigate={setCurrentPage}>
        {renderPage()}
      </Layout>
    </ModelListProvider>
  );
}

export default App;
