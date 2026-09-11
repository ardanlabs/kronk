import { api } from '../services/api';
import { LibsManager } from './BuckyLibs';

const malinaBackend = {
  id: 'malina',
  engine: 'Stable-diffusion.cpp',
  backend: 'Malina',
  environmentVariable: 'KRONK_MALINA_LIB_PATH',
  rootFolder: 'malina-libraries',
  getVersion: () => api.getMalinaLibsVersion(),
  getCombinations: () => api.getMalinaLibsCombinations(),
  listInstalls: () => api.listMalinaLibsInstalls(),
  removeInstall: (arch: string, os: string, processor: string) => api.removeMalinaLibsInstall(arch, os, processor),
  pull: (
    onMessage: Parameters<typeof api.pullMalinaLibs>[0],
    onError: Parameters<typeof api.pullMalinaLibs>[1],
    onComplete: Parameters<typeof api.pullMalinaLibs>[2],
    opts: Parameters<typeof api.pullMalinaLibs>[3],
  ) => api.pullMalinaLibs(onMessage, onError, onComplete, opts),
};

export default function MalinaLibs() {
  return <LibsManager backend={malinaBackend} />;
}
