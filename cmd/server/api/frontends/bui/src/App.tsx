import { BrowserRouter, Routes, Route } from 'react-router-dom';
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
import Settings from './components/Settings';
import DocsSDK from './components/DocsSDK';
import DocsSDKKronk from './components/DocsSDKKronk';
import DocsSDKModel from './components/DocsSDKModel';
import DocsSDKExamples from './components/DocsSDKExamples';
import DocsCLICatalog from './components/DocsCLICatalog';
import DocsCLILibs from './components/DocsCLILibs';
import DocsCLIModel from './components/DocsCLIModel';
import DocsCLISecurity from './components/DocsCLISecurity';
import DocsCLIServer from './components/DocsCLIServer';
import DocsAPIChat from './components/DocsAPIChat';
import DocsAPIEmbeddings from './components/DocsAPIEmbeddings';
import DocsAPITools from './components/DocsAPITools';
import { ModelListProvider } from './contexts/ModelListContext';
import { TokenProvider } from './contexts/TokenContext';

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
  | 'settings'
  | 'docs-sdk'
  | 'docs-sdk-kronk'
  | 'docs-sdk-model'
  | 'docs-sdk-examples'
  | 'docs-cli-catalog'
  | 'docs-cli-libs'
  | 'docs-cli-model'
  | 'docs-cli-security'
  | 'docs-cli-server'
  | 'docs-api-chat'
  | 'docs-api-embeddings'
  | 'docs-api-tools';

export const routeMap: Record<Page, string> = {
  'home': '/',
  'model-list': '/models',
  'model-ps': '/models/running',
  'model-pull': '/models/pull',
  'catalog-list': '/catalog',
  'libs-pull': '/libs/pull',
  'security-key-list': '/security/keys',
  'security-key-create': '/security/keys/create',
  'security-key-delete': '/security/keys/delete',
  'security-token-create': '/security/tokens/create',
  'settings': '/settings',
  'docs-sdk': '/docs/sdk',
  'docs-sdk-kronk': '/docs/sdk/kronk',
  'docs-sdk-model': '/docs/sdk/model',
  'docs-sdk-examples': '/docs/sdk/examples',
  'docs-cli-catalog': '/docs/cli/catalog',
  'docs-cli-libs': '/docs/cli/libs',
  'docs-cli-model': '/docs/cli/model',
  'docs-cli-security': '/docs/cli/security',
  'docs-cli-server': '/docs/cli/server',
  'docs-api-chat': '/docs/api/chat',
  'docs-api-embeddings': '/docs/api/embeddings',
  'docs-api-tools': '/docs/api/tools',
};

export const pathToPage: Record<string, Page> = Object.fromEntries(
  Object.entries(routeMap).map(([page, path]) => [path, page as Page])
);

function HomePage() {
  return (
    <div className="welcome">
      <img
        src="https://raw.githubusercontent.com/ardanlabs/kronk/refs/heads/main/images/project/kronk_banner.jpg"
        alt="Kronk Banner"
        className="welcome-banner"
      />
      <h2>Welcome to Kronk</h2>
      <p>Select an option from the sidebar to manage your Kronk environment.</p>
    </div>
  );
}

function App() {
  return (
    <BrowserRouter>
      <TokenProvider>
        <ModelListProvider>
          <Layout>
            <Routes>
            <Route path="/" element={<HomePage />} />
            <Route path="/models" element={<ModelList />} />
            <Route path="/models/running" element={<ModelPs />} />
            <Route path="/models/pull" element={<ModelPull />} />
            <Route path="/catalog" element={<CatalogList />} />
            <Route path="/libs/pull" element={<LibsPull />} />
            <Route path="/security/keys" element={<SecurityKeyList />} />
            <Route path="/security/keys/create" element={<SecurityKeyCreate />} />
            <Route path="/security/keys/delete" element={<SecurityKeyDelete />} />
            <Route path="/security/tokens/create" element={<SecurityTokenCreate />} />
            <Route path="/settings" element={<Settings />} />
            <Route path="/docs/sdk" element={<DocsSDK />} />
            <Route path="/docs/sdk/kronk" element={<DocsSDKKronk />} />
            <Route path="/docs/sdk/model" element={<DocsSDKModel />} />
            <Route path="/docs/sdk/examples" element={<DocsSDKExamples />} />
            <Route path="/docs/cli/catalog" element={<DocsCLICatalog />} />
            <Route path="/docs/cli/libs" element={<DocsCLILibs />} />
            <Route path="/docs/cli/model" element={<DocsCLIModel />} />
            <Route path="/docs/cli/security" element={<DocsCLISecurity />} />
            <Route path="/docs/cli/server" element={<DocsCLIServer />} />
            <Route path="/docs/api/chat" element={<DocsAPIChat />} />
            <Route path="/docs/api/embeddings" element={<DocsAPIEmbeddings />} />
            <Route path="/docs/api/tools" element={<DocsAPITools />} />
          </Routes>
          </Layout>
        </ModelListProvider>
      </TokenProvider>
    </BrowserRouter>
  );
}

export default App;
