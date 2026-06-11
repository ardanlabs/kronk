import ModelPlayground from './ModelPlayground';

// Basic verifies a model performs the basic tasks it needs to (manual chat /
// tools), backed by the shared testing engine locked to manual mode.
export default function TestingBasic() {
  return <ModelPlayground mode="manual" />;
}
