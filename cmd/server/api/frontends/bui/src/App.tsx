import { useState } from 'react';
import Layout from './components/Layout';
import ModelList from './components/ModelList';
import ModelPs from './components/ModelPs';


import ModelPull from './components/ModelPull';

import CatalogList from './components/CatalogList';


import LibsPull from './components/LibsPull';
import SecurityKeyList from './components/SecurityKeyList';
import SecurityKeyCreate from './components/SecurityKeyCreate';
import SecurityKeyDelete from './components/SecurityKeyDelete';
import SecurityTokenCreate from './components/SecurityTokenCreate';
import DocsSDK from './components/DocsSDK';
import DocsSDKKronk from './components/DocsSDKKronk';
import DocsSDKModel from './components/DocsSDKModel';
import DocsCLI from './components/DocsCLI';
import DocsWebAPI from './components/DocsWebAPI';
import { ModelListProvider } from './contexts/ModelListContext';

export type Page =
  | 'home'
  | 'model-list'
  | 'model-ps'
  | 'model-pull'
  | 'catalog-list'
  | 'libs-pull'
  | 'security-key-list'
  | 'security-key-create'
  | 'security-key-delete'
  | 'security-token-create'
  | 'docs-sdk'
  | 'docs-sdk-kronk'
  | 'docs-sdk-model'
  | 'docs-cli'
  | 'docs-webapi';

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
      case 'catalog-list':
        return <CatalogList />;
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
      case 'docs-sdk':
        return <DocsSDK />;
      case 'docs-sdk-kronk':
        return <DocsSDKKronk />;
      case 'docs-sdk-model':
        return <DocsSDKModel />;
      case 'docs-cli':
        return <DocsCLI />;
      case 'docs-webapi':
        return <DocsWebAPI />;
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
