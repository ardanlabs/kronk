import ModelPlayground from './ModelPlayground';

// Configuration identifies the best config parameters for a model, backed by
// the shared testing engine locked to the config sweep.
export default function TestingConfiguration() {
  return <ModelPlayground mode="config" />;
}
