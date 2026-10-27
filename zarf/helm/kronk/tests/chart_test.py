"""Check the chart's rendered public contract; no cluster or Go runtime needed."""

import pathlib
import subprocess
import tempfile
import unittest

import yaml

CHART = pathlib.Path(__file__).resolve().parents[1]


def render(values=None, release="kronk"):
    result = subprocess.run(
        ["helm", "template", release, str(CHART), "-f", "-"],
        input=yaml.safe_dump(values or {}), text=True, capture_output=True,
    )
    if result.returncode:
        raise ValueError(result.stderr)
    return [doc for doc in yaml.safe_load_all(result.stdout) if doc]


def resource(docs, kind):
    return next(doc for doc in docs if doc["kind"] == kind)


class ChartTest(unittest.TestCase):
    def test_defaults_and_variants(self):
        version = yaml.safe_load((CHART / "Chart.yaml").read_text())["appVersion"]
        for variant in ("cpu", "cuda", "vulkan", "rocm", "all"):
            with self.subTest(variant=variant):
                docs = render({"image": {"variant": variant}})
                sts = resource(docs, "StatefulSet")["spec"]
                self.assertEqual(sts["replicas"], 1)
                pod = sts["template"]["spec"]
                container = pod["containers"][0]
                self.assertEqual(container["image"], f"ghcr.io/ardanlabs/kronk:v{version}-{variant}")
                env = {entry["name"]: entry.get("value") for entry in container["env"]}
                self.assertEqual(env["KRONK_AUTHORIZATION_MODE"], "authenticated")
                self.assertEqual(env["KRONK_WEB_ADMIN_ENABLED"], "false")
                self.assertEqual(env.get("KRONK_PROCESSOR"), None if variant == "all" else variant)
                self.assertEqual(env["KRONK_WEB_DEBUG_HOST"], "127.0.0.1:11445")
                self.assertFalse(pod["automountServiceAccountToken"])
                self.assertTrue(pod["securityContext"]["runAsNonRoot"])
                for item in pod["containers"] + pod["initContainers"]:
                    self.assertFalse(item["securityContext"]["allowPrivilegeEscalation"])
                    self.assertEqual(item["securityContext"]["capabilities"]["drop"], ["ALL"])
                self.assertEqual(container["readinessProbe"]["httpGet"]["path"], "/v1/readiness")

    def test_metrics_remain_private_with_public_api(self):
        docs = render({"service": {"type": "LoadBalancer", "port": 8080},
                       "metrics": {"enabled": True, "port": 9090, "serviceMonitor": {"enabled": True}}})
        services = {doc["metadata"]["name"]: doc for doc in docs if doc["kind"] == "Service"}
        api = services["kronk"]["spec"]
        self.assertEqual(api["type"], "LoadBalancer")
        self.assertEqual([p["port"] for p in api["ports"]], [8080])
        self.assertEqual(services["kronk-metrics"]["spec"]["type"], "ClusterIP")
        monitor = resource(docs, "ServiceMonitor")["spec"]
        selector = monitor["selector"]["matchLabels"]
        selected = [s for s in services.values()
                    if all(s["metadata"]["labels"].get(k) == v for k, v in selector.items())]
        self.assertEqual([s["metadata"]["name"] for s in selected], ["kronk-metrics"])
        hook = resource(docs, "Pod")
        self.assertIn("http://kronk:8080/v1/liveness", hook["spec"]["containers"][0]["args"])

    def test_long_release_names(self):
        docs = render({"metrics": {"enabled": True}, "modelConfig": {"enabled": True}}, release="a" * 53)
        names = {doc["metadata"]["name"] for doc in docs}
        self.assertTrue(all(len(name) <= 63 for name in names))
        sts = resource(docs, "StatefulSet")["spec"]
        self.assertIn(sts["serviceName"], names)

    def test_storage_and_config(self):
        for storage_class, expected in (("", None), ("-", ""), ("fast-ssd", "fast-ssd")):
            docs = render({"persistence": {"storageClass": storage_class}})
            claim = resource(docs, "StatefulSet")["spec"]["volumeClaimTemplates"][0]["spec"]
            self.assertEqual(claim.get("storageClassName"), expected)
        docs = render({"persistence": {"existingClaim": "my-models"},
                       "modelConfig": {"enabled": True, "existingConfigMap": "my-config"}})
        sts = resource(docs, "StatefulSet")["spec"]
        self.assertNotIn("volumeClaimTemplates", sts)
        volumes = {v["name"]: v for v in sts["template"]["spec"]["volumes"]}
        self.assertEqual(volumes["data"]["persistentVolumeClaim"]["claimName"], "my-models")
        self.assertEqual(volumes["model-config"]["configMap"]["name"], "my-config")
        self.assertFalse(any(d["kind"] == "ConfigMap" for d in docs))

    def test_options_and_environment(self):
        values = yaml.safe_load((CHART / "examples/cuda.yaml").read_text())
        values.update({"image": {"variant": "cuda", "tag": "custom-cuda"},
                       "runtimeClassName": "nvidia", "auth": {"existingSecret": "signing-keys"},
                       "env": {"KRONK_WEB_ADMIN_ENABLED": True, "KRONK_POOL_BUDGET_PERCENT": 80},
                       "extraEnv": [{"name": "KRONK_HF_TOKEN", "valueFrom": {
                           "secretKeyRef": {"name": "hf", "key": "token"}}},
                           {"name": "NVIDIA_DRIVER_CAPABILITIES", "value": "compute,utility,graphics"}],
                       "ingress": {"enabled": True, "className": "nginx", "tls": [{
                           "hosts": ["kronk.example.com"], "secretName": "tls"}]}})
        docs = render(values)
        pod = resource(docs, "StatefulSet")["spec"]["template"]["spec"]
        server = pod["containers"][0]
        self.assertEqual(server["resources"]["limits"]["nvidia.com/gpu"], 1)
        self.assertEqual(pod["runtimeClassName"], "nvidia")
        self.assertEqual(server["image"], "ghcr.io/ardanlabs/kronk:custom-cuda")
        env = server["env"]
        self.assertEqual(len({e["name"] for e in env}), len(env))
        self.assertIn({"name": "KRONK_WEB_ADMIN_ENABLED", "value": "true"}, env)
        self.assertIn(values["extraEnv"][1], env)
        pull = next(c for c in pod["initContainers"] if c["name"] == "pull-models")
        self.assertIn("unsloth/Qwen3-0.6B-Q8_0", pull["args"])
        self.assertIn(values["extraEnv"][0], pull["env"])
        self.assertEqual(resource(docs, "Ingress")["spec"]["ingressClassName"], "nginx")

    def test_config_changes_roll_the_pod(self):
        checksums = []
        for budget in (60, 80):
            docs = render({"modelConfig": {"enabled": True, "content":
                f"version: 2\nkms:\n  pool:\n    budget-percent: {budget}\nmodels: {{}}\n"}})
            template = resource(docs, "StatefulSet")["spec"]["template"]
            checksums.append(template["metadata"]["annotations"]["checksum/config"])
        self.assertNotEqual(*checksums)

    def test_invalid_values_fail(self):
        cases = [
            {"replicaCount": 2}, {"image": {"variant": "metal"}},
            {"auth": {"mode": "oops"}}, {"auth": {"masterPem": "not-supported"}},
            {"metrics": {"serviceMonitor": {"enabled": True}}},
            {"env": {"KRONK_BASE_PATH": "/elsewhere"}},
            {"extraEnv": [{"name": "KRONK_AUTHORIZATION_MODE", "value": "open"}]},
            {"env": {"KRONK_HF_TOKEN": "x"}, "extraEnv": [{"name": "KRONK_HF_TOKEN", "value": "y"}]},
        ]
        for values in cases:
            with self.subTest(values=values), self.assertRaises(ValueError):
                render(values)

    def test_seed_replaces_image_bundles_and_reconciles_secret_keys(self):
        docs = render({"auth": {"existingSecret": "keys"}})
        pod = resource(docs, "StatefulSet")["spec"]["template"]["spec"]
        script = pod["initContainers"][0]["args"][0]
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            for name in ("image/models", "data", "secret"):
                (root / name).mkdir(parents=True)
            (root / "image/models/model_config.yaml").write_text("original config")
            for key in ("master.pem", "master.jwt", "retired.pem"):
                (root / "secret" / key).write_text("fixture " + key)
            script = script.replace("/kronk", str(root / "image"))
            script = script.replace("/data", str(root / "data"))
            script = script.replace("/secrets/keys", str(root / "secret"))
            for arch, content in (("amd64", "original"), ("arm64", "new architecture"), ("amd64", "updated")):
                for name in ("libraries", "bucky-libraries", "malina-libraries"):
                    bundle = pathlib.Path(name) / "linux" / arch / "cpu" / "bundle.so"
                    source = root / "image" / bundle
                    source.parent.mkdir(parents=True, exist_ok=True)
                    source.write_text(content)
                    stale = root / "data" / bundle.parent / "obsolete.so"
                    stale.parent.mkdir(parents=True, exist_ok=True)
                    stale.write_text("old image file")
                subprocess.run(["/bin/sh", "-c", script], check=True, capture_output=True)
                for name in ("libraries", "bucky-libraries", "malina-libraries"):
                    self.assertEqual((root / "data" / name / "linux" / arch / "cpu/bundle.so").read_text(), content)
                    self.assertFalse((root / "data" / name / "linux" / arch / "cpu/obsolete.so").exists())
                newest = max((root / "data/keys").glob("*.pem"), key=lambda p: p.stat().st_mtime)
                self.assertEqual(newest.name, "retired.pem")
            (root / "secret/retired.pem").unlink()
            (root / "data/models/model_config.yaml").write_text("operator config")
            subprocess.run(["/bin/sh", "-c", script], check=True, capture_output=True)
            self.assertFalse((root / "data/keys/retired.pem").exists())
            self.assertEqual((root / "data/keys/master.pem").read_text(), "fixture master.pem")
            self.assertEqual((root / "data/keys/master.pem").stat().st_mode & 0o777, 0o600)
            self.assertEqual((root / "data/models/model_config.yaml").read_text(), "operator config")


if __name__ == "__main__":
    unittest.main(verbosity=2)
