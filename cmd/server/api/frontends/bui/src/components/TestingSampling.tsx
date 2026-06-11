import ModelPlayground from './ModelPlayground';

// Sampling identifies the best sampling parameters for a model, backed by the
// shared testing engine locked to the sampling sweep.
export default function TestingSampling() {
  return <ModelPlayground mode="sampling" />;
}
