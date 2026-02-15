import type {
  ListModelInfoResponse,
  ModelDetailsResponse,
  ModelInfoResponse,
  CatalogModelsResponse,
  CatalogModelResponse,
  KeysResponse,
  TokenRequest,
  TokenResponse,
  PullResponse,
  AsyncPullResponse,
  VersionResponse,
  ChatRequest,
  ChatStreamResponse,
  VRAMRequest,
  VRAMCalculatorResponse,
  HFLookupResponse,
  SaveCatalogRequest,
  SaveCatalogResponse,
  CatalogFileInfo,
  PublishCatalogResponse,
  RepoPathResponse,
} from '../types';

class ApiService {
  private baseUrl = '/v1';

  private async request<T>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<T> {
    const response = await fetch(`${this.baseUrl}${endpoint}`, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        ...options.headers,
      },
    });

    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.error?.message || `HTTP ${response.status}`);
    }

    if (response.status === 204) {
      return undefined as T;
    }

    return response.json();
  }

  async listModels(): Promise<ListModelInfoResponse> {
    return this.request<ListModelInfoResponse>('/models');
  }

  async listModelsExtended(): Promise<ListModelInfoResponse> {
    return this.request<ListModelInfoResponse>('/models?extended-config=true');
  }

  async rebuildModelIndex(): Promise<void> {
    const response = await fetch(`${this.baseUrl}/models/index`, {
      method: 'POST',
    });
    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.error?.message || `HTTP ${response.status}`);
    }
  }

  async listRunningModels(): Promise<ModelDetailsResponse> {
    return this.request<ModelDetailsResponse>('/models/ps');
  }

  async showModel(id: string): Promise<ModelInfoResponse> {
    return this.request<ModelInfoResponse>(`/models/${encodeURIComponent(id)}`);
  }

  async getLibsVersion(): Promise<VersionResponse> {
    return this.request<VersionResponse>('/libs');
  }

  async pullModelAsync(modelUrl: string): Promise<AsyncPullResponse> {
    const response = await fetch(`${this.baseUrl}/models/pull`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ model_url: modelUrl, async: true }),
    });

    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.error?.message || `HTTP ${response.status}`);
    }

    return response.json();
  }

  streamPullSession(
    sessionId: string,
    onMessage: (data: PullResponse) => void,
    onError: (error: string) => void,
    onComplete: () => void
  ): () => void {
    const controller = new AbortController();

    fetch(`${this.baseUrl}/models/pull/${encodeURIComponent(sessionId)}`, {
      method: 'GET',
      signal: controller.signal,
    })
      .then(async (response) => {
        if (response.status === 400) {
          onError('Session closed');
          return;
        }
        if (!response.ok) {
          onError(`HTTP ${response.status}`);
          return;
        }

        const reader = response.body?.getReader();
        if (!reader) {
          onError('Streaming not supported');
          return;
        }

        const decoder = new TextDecoder();
        let buffer = '';

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split('\n');
          buffer = lines.pop() || '';

          for (const line of lines) {
            if (!line.trim()) continue;
            const jsonStr = line.startsWith('data: ') ? line.slice(6) : line;
            if (!jsonStr.trim()) continue;
            try {
              const data = JSON.parse(jsonStr) as PullResponse;
              onMessage(data);
              if (data.status === 'downloaded' || data.downloaded) {
                onComplete();
                return;
              }
            } catch {
              onError('Failed to parse response');
            }
          }
        }

        onComplete();
      })
      .catch((err) => {
        if (err.name !== 'AbortError') {
          onError('Connection error');
        }
      });

    return () => controller.abort();
  }

  pullModel(
    modelUrl: string,
    projUrl: string | undefined,
    onMessage: (data: PullResponse) => void,
    onError: (error: string) => void,
    onComplete: () => void
  ): () => void {
    const controller = new AbortController();

    const body: { model_url: string; proj_url?: string } = { model_url: modelUrl };
    if (projUrl) {
      body.proj_url = projUrl;
    }

    fetch(`${this.baseUrl}/models/pull`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      signal: controller.signal,
    })
      .then(async (response) => {
        if (!response.ok) {
          onError(`HTTP ${response.status}`);
          return;
        }

        const reader = response.body?.getReader();
        if (!reader) {
          onError('Streaming not supported');
          return;
        }

        const decoder = new TextDecoder();
        let buffer = '';

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split('\n');
          buffer = lines.pop() || '';

          for (const line of lines) {
            if (!line.trim()) continue;
            const jsonStr = line.startsWith('data: ') ? line.slice(6) : line;
            if (!jsonStr.trim()) continue;
            try {
              const data = JSON.parse(jsonStr) as PullResponse;
              onMessage(data);
              if (data.status === 'complete' || data.downloaded) {
                onComplete();
                return;
              }
            } catch {
              onError('Failed to parse response');
            }
          }
        }

        onComplete();
      })
      .catch((err) => {
        if (err.name !== 'AbortError') {
          onError('Connection error');
        }
      });

    return () => controller.abort();
  }

  async removeModel(id: string): Promise<void> {
    await this.request(`/models/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    });
  }

  async listCatalog(): Promise<CatalogModelsResponse> {
    return this.request<CatalogModelsResponse>('/catalog');
  }

  async showCatalogModel(id: string): Promise<CatalogModelResponse> {
    return this.request<CatalogModelResponse>(`/catalog/${encodeURIComponent(id)}`);
  }

  pullCatalogModel(
    id: string,
    onMessage: (data: PullResponse) => void,
    onError: (error: string) => void,
    onComplete: () => void
  ): () => void {
    const controller = new AbortController();

    fetch(`${this.baseUrl}/catalog/pull/${encodeURIComponent(id)}`, {
      method: 'POST',
      signal: controller.signal,
    })
      .then(async (response) => {
        if (!response.ok) {
          onError(`HTTP ${response.status}`);
          return;
        }

        const reader = response.body?.getReader();
        if (!reader) {
          onError('Streaming not supported');
          return;
        }

        const decoder = new TextDecoder();
        let buffer = '';

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split('\n');
          buffer = lines.pop() || '';

          for (const line of lines) {
            if (!line.trim()) continue;
            const jsonStr = line.startsWith('data: ') ? line.slice(6) : line;
            if (!jsonStr.trim()) continue;
            try {
              const data = JSON.parse(jsonStr) as PullResponse;
              onMessage(data);
              if (data.status === 'complete' || data.downloaded) {
                onComplete();
                return;
              }
            } catch {
              onError('Failed to parse response');
            }
          }
        }

        onComplete();
      })
      .catch((err) => {
        if (err.name !== 'AbortError') {
          onError('Connection error');
        }
      });

    return () => controller.abort();
  }

  pullLibs(
    onMessage: (data: VersionResponse) => void,
    onError: (error: string) => void,
    onComplete: () => void,
    allowUpgrade?: boolean
  ): () => void {
    const controller = new AbortController();

    const url = allowUpgrade ? `${this.baseUrl}/libs/pull?allow-upgrade=true` : `${this.baseUrl}/libs/pull`;

    fetch(url, {
      method: 'POST',
      signal: controller.signal,
    })
      .then(async (response) => {
        if (!response.ok) {
          onError(`HTTP ${response.status}`);
          return;
        }

        const reader = response.body?.getReader();
        if (!reader) {
          onError('Streaming not supported');
          return;
        }

        const decoder = new TextDecoder();
        let buffer = '';

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split('\n');
          buffer = lines.pop() || '';

          for (const line of lines) {
            if (!line.trim()) continue;
            const jsonStr = line.startsWith('data: ') ? line.slice(6) : line;
            if (!jsonStr.trim()) continue;
            try {
              const data = JSON.parse(jsonStr) as VersionResponse;
              onMessage(data);
              if (data.status === 'complete') {
                onComplete();
                return;
              }
            } catch {
              onError('Failed to parse response');
            }
          }
        }

        onComplete();
      })
      .catch((err) => {
        if (err.name !== 'AbortError') {
          onError('Connection error');
        }
      });

    return () => controller.abort();
  }

  async listKeys(token: string): Promise<KeysResponse> {
    return this.request<KeysResponse>('/security/keys', {
      headers: {
        Authorization: `Bearer ${token}`,
      },
    });
  }

  async createKey(token: string): Promise<{ id: string }> {
    return this.request<{ id: string }>('/security/keys/add', {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${token}`,
      },
    });
  }

  async deleteKey(token: string, keyId: string): Promise<void> {
    await this.request(`/security/keys/remove/${encodeURIComponent(keyId)}`, {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${token}`,
      },
    });
  }

  async createToken(token: string, request: TokenRequest): Promise<TokenResponse> {
    return this.request<TokenResponse>('/security/token/create', {
      method: 'POST',
      headers: {
        Authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(request),
    });
  }

  streamChat(
    request: ChatRequest,
    onMessage: (data: ChatStreamResponse) => void,
    onError: (error: string) => void,
    onComplete: () => void
  ): () => void {
    const controller = new AbortController();

    fetch(`${this.baseUrl}/chat/completions`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ...request, stream: true }),
      signal: controller.signal,
    })
      .then(async (response) => {
        if (!response.ok) {
          const error = await response.json();
          onError(error.error?.message || `HTTP ${response.status}`);
          return;
        }

        const reader = response.body?.getReader();
        if (!reader) {
          onError('Streaming not supported');
          return;
        }

        const decoder = new TextDecoder();
        let buffer = '';

        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split('\n');
          buffer = lines.pop() || '';

          for (const line of lines) {
            if (!line.trim() || line === 'data: [DONE]') continue;
            const jsonStr = line.startsWith('data: ') ? line.slice(6) : line;
            if (!jsonStr.trim()) continue;
            try {
              const data = JSON.parse(jsonStr) as ChatStreamResponse;
              onMessage(data);
            } catch {
              // Skip malformed JSON
            }
          }
        }

        onComplete();
      })
      .catch((err) => {
        if (err.name !== 'AbortError') {
          onError(err.message || 'Connection error');
        }
      });

    return () => controller.abort();
  }

  async lookupHuggingFace(input: string): Promise<HFLookupResponse> {
    return this.request<HFLookupResponse>('/catalog/lookup', {
      method: 'POST',
      body: JSON.stringify({ input }),
    });
  }

  async saveCatalogModel(data: SaveCatalogRequest): Promise<SaveCatalogResponse> {
    return this.request<SaveCatalogResponse>('/catalog/save', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  async deleteCatalogModel(id: string): Promise<SaveCatalogResponse> {
    return this.request<SaveCatalogResponse>(`/catalog/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    });
  }

  async listCatalogFiles(): Promise<CatalogFileInfo[]> {
    return this.request<CatalogFileInfo[]>('/catalog-files');
  }

  async publishCatalogModel(catalogFile: string): Promise<PublishCatalogResponse> {
    return this.request<PublishCatalogResponse>('/catalog/publish', {
      method: 'POST',
      body: JSON.stringify({ catalog_file: catalogFile }),
    });
  }

  async getCatalogRepoPath(): Promise<RepoPathResponse> {
    return this.request<RepoPathResponse>('/catalog-repo-path');
  }

  async calculateVRAM(request: VRAMRequest, token?: string): Promise<VRAMCalculatorResponse> {
    const headers: Record<string, string> = {};
    if (token) {
      headers['Authorization'] = `Bearer ${token}`;
    }
    return this.request<VRAMCalculatorResponse>('/models/vram', {
      method: 'POST',
      headers,
      body: JSON.stringify(request),
    });
  }

  async listGrammars(): Promise<{ files: string[] }> {
    return this.request<{ files: string[] }>('/grammars');
  }

  async getGrammarContent(name: string): Promise<{ content: string }> {
    return this.request<{ content: string }>(`/grammars/${encodeURIComponent(name)}`);
  }

  async listTemplates(): Promise<{ files: string[] }> {
    return this.request<{ files: string[] }>('/templates');
  }

}

export const api = new ApiService();
