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
import Chat from './components/Chat';
import DocsSDK from './components/DocsSDK';
import DocsSDKKronk from './components/DocsSDKKronk';
import DocsSDKModel from './components/DocsSDKModel';
import DocsSDKExamples from './components/DocsSDKExamples';
import DocsCLICatalog from './components/DocsCLICatalog';
import DocsCLILibs from './components/DocsCLILibs';
import DocsCLIModel from './components/DocsCLIModel';
import DocsCLIRun from './components/DocsCLIRun';
import DocsCLISecurity from './components/DocsCLISecurity';
import DocsCLIServer from './components/DocsCLIServer';
import DocsAPIChat from './components/DocsAPIChat';
import DocsAPIMessages from './components/DocsAPIMessages';
import DocsAPIResponses from './components/DocsAPIResponses';
import DocsAPIEmbeddings from './components/DocsAPIEmbeddings';
import DocsAPIRerank from './components/DocsAPIRerank';
import DocsAPITools from './components/DocsAPITools';
import { ModelListProvider } from './contexts/ModelListContext';
import { TokenProvider } from './contexts/TokenContext';
import { DownloadProvider } from './contexts/DownloadContext';

export type Page =
  | 'home'
  | 'chat'
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
  | 'docs-cli-run'
  | 'docs-cli-security'
  | 'docs-cli-server'
  | 'docs-api-chat'
  | 'docs-api-messages'
  | 'docs-api-responses'
  | 'docs-api-embeddings'
  | 'docs-api-rerank'
  | 'docs-api-tools';

export const routeMap: Record<Page, string> = {
  'home': '/',
  'chat': '/chat',
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
  'docs-cli-run': '/docs/cli/run',
  'docs-cli-security': '/docs/cli/security',
  'docs-cli-server': '/docs/cli/server',
  'docs-api-chat': '/docs/api/chat',
  'docs-api-messages': '/docs/api/messages',
  'docs-api-responses': '/docs/api/responses',
  'docs-api-embeddings': '/docs/api/embeddings',
  'docs-api-rerank': '/docs/api/rerank',
  'docs-api-tools': '/docs/api/tools',
};

export const pathToPage: Record<string, Page> = Object.fromEntries(
  Object.entries(routeMap).map(([page, path]) => [path, page as Page])
);

function HomePage() {
  return (
    <div className="home-page">
      <div className="hero-section">
        <img
          src="https://raw.githubusercontent.com/ardanlabs/kronk/refs/heads/main/images/project/kronk_banner.jpg"
          alt="Kronk Banner"
          className="hero-banner"
        />
        <p className="hero-tagline">
          Hardware-accelerated local inference with llama.cpp directly integrated into your Go applications
        </p>
      </div>

      <div className="features-grid">
        <div className="feature-card">
          <div className="feature-icon">🚀</div>
          <h3>High-Level Go API</h3>
          <p>Feels similar to using an OpenAI compatible API, but runs entirely on your hardware</p>
        </div>
        <div className="feature-card">
          <div className="feature-icon">🔧</div>
          <h3>OpenAI Compatible Server</h3>
          <p>Model server for chat completions and embeddings, compatible with OpenWebUI</p>
        </div>
        <div className="feature-card">
          <div className="feature-icon">🎯</div>
          <h3>Multimodal Support</h3>
          <p>Text, vision, and audio models with full hardware acceleration</p>
        </div>
        <div className="feature-card">
          <div className="feature-icon">⚡</div>
          <h3>GPU Acceleration</h3>
          <p>Metal on macOS, CUDA/Vulkan/ROCm on Linux, CUDA/Vulkan on Windows</p>
        </div>
      </div>

      <div className="home-cta">
        <p>Use the sidebar to manage models, browse the catalog, or explore the SDK documentation.</p>
      </div>
    </div>
  );
}

function App() {
  return (
    <BrowserRouter>
      <TokenProvider>
        <ModelListProvider>
          <DownloadProvider>
            <Layout>
              <Routes>
                <Route path="/" element={<HomePage />} />
                <Route path="/chat" element={<Chat />} />
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
                <Route path="/docs/cli/run" element={<DocsCLIRun />} />
                <Route path="/docs/cli/security" element={<DocsCLISecurity />} />
                <Route path="/docs/cli/server" element={<DocsCLIServer />} />
                <Route path="/docs/api/chat" element={<DocsAPIChat />} />
                <Route path="/docs/api/messages" element={<DocsAPIMessages />} />
                <Route path="/docs/api/responses" element={<DocsAPIResponses />} />
                <Route path="/docs/api/embeddings" element={<DocsAPIEmbeddings />} />
                <Route path="/docs/api/rerank" element={<DocsAPIRerank />} />
                <Route path="/docs/api/tools" element={<DocsAPITools />} />
              </Routes>
            </Layout>
          </DownloadProvider>
        </ModelListProvider>
      </TokenProvider>
    </BrowserRouter>
  );
}

export default App;
