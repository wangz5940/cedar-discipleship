import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


SCRIPTS = Path(__file__).resolve().parent


class DeployScriptsTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        shutil.copytree(SCRIPTS, self.root / "scripts", ignore=shutil.ignore_patterns("__pycache__"))
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.log = self.root / "commands.jsonl"
        self.env = {
            "PATH": str(self.bin) + os.pathsep + os.environ["PATH"],
            "HOME": str(self.root), "ENV_FILE": str(self.root / ".env"),
            "COMPOSE_PROJECT_NAME": "test", "COMMAND_LOG": str(self.log),
            "AGP_DATA_DIR": str(self.root / "data"), "AGP_LOG_DIR": str(self.root / "logs"),
            "MIGRATION_REPORT_DIR": str(self.root / "reports"),
            "AGP_DEPLOY_LOCK_DIR": str(self.root / "deploy.lock"),
            "AGP_DOCKER_USE_SUDO": "false",
        }
        (self.root / ".env").write_text("")
        self.executable("docker", """import json, os, sys
args = sys.argv[1:]
images = ""
if "up" in args and "-f" in args:
    path = args[len(args) - 1 - args[::-1].index("-f") + 1]
    if os.path.isfile(path):
        images = open(path).read()
with open(os.environ["COMMAND_LOG"], "a") as log:
    log.write(json.dumps({"args": args, "jwt_length": len(os.getenv("AGP_JWT_SECRET", "")),
                          "password_length": len(os.getenv("BOOTSTRAP_SUPERADMIN_PASSWORD", "")),
                          "images": images, "web_bind_host": os.getenv("AGP_WEB_BIND_HOST", "")}) + "\\n")
if args[:2] == ["image", "inspect"]:
    if "org.opencontainers.image.revision" in args[args.index("--format") + 1]:
        print(os.environ.get("IMAGE_REVISION", "a" * 40))
    else:
        print("sha256:" + ("b" if "backend" in args[-1] else "c") * 64)
if "--dry-run=true" in args and os.environ.get("FAIL_DRY_RUN") == "1":
    sys.exit(1)
""")

    def executable(self, name, body):
        path = self.bin / name
        path.write_text("#!" + shutil.which("python3") + "\n" + body)
        path.chmod(0o700)
        return str(path)

    def run_script(self, script):
        return subprocess.run(["bash", str(self.root / "scripts" / script)],
                              env=self.env, capture_output=True, text=True, timeout=20)

    def commands(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def test_oneclick_generates_default_credentials_under_pipefail(self):
        self.env.update(RUN_PRIMARY_MIGRATION="false", RUN_RESOURCE_FILE_MIGRATION="false")
        result = self.run_script("deploy-oneclick.sh")
        self.assertEqual(result.returncode, 0, result.stderr)
        up = next(item for item in self.commands() if "up" in item["args"])
        self.assertEqual(up["jwt_length"], 48)
        self.assertEqual(up["password_length"], 16)

    def test_oneclick_reads_frontend_bind_host_from_environment_file(self):
        self.env.update(RUN_PRIMARY_MIGRATION="false", RUN_RESOURCE_FILE_MIGRATION="false")
        (self.root / ".env").write_text("AGP_WEB_BIND_HOST=127.0.0.1\n")
        result = self.run_script("deploy-oneclick.sh")
        self.assertEqual(result.returncode, 0, result.stderr)
        up = next(item for item in self.commands() if "up" in item["args"])
        self.assertEqual(up["web_bind_host"], "127.0.0.1")

    def test_finite_random_input_with_and_without_openssl(self):
        for fallback in (False, True):
            with self.subTest(fallback=fallback):
                script = """
set -euo pipefail
if [ "$1" = fallback ]; then
  command() {
    if [ "$1" = -v ] && [ "$2" = openssl ]; then return 1; fi
    builtin command "$@"
  }
fi
. "$2"
hex="$(rand_hex 24)"
password="$(rand_password 32)"
[[ "$hex" =~ ^[a-f0-9]{48}$ ]]
[[ "$password" =~ ^[A-Za-z0-9]{32}$ ]]
"""
                result = subprocess.run(
                    ["bash", "-c", script, "random-test", "fallback" if fallback else "openssl",
                     str(self.root / "scripts/lib/random.sh")],
                    env=self.env, capture_output=True, text=True, timeout=10)
                self.assertEqual(result.returncode, 0, result.stderr)

    def test_group_migration_generates_password_and_stops_after_failed_dry_run(self):
        (self.root / "config.json").write_text("{}")
        (self.root / "records.json").write_text("[]")
        self.env.update(GROUP_CODE="test", GROUP_NAME="Test", EXECUTE_IMPORT="true", FAIL_DRY_RUN="1",
                        CONFIG_PATH=str(self.root / "config.json"), RECORDS_PATH=str(self.root / "records.json"),
                        REPORT_DIR=str(self.root / "reports"))
        result = self.run_script("migrate-group.sh")
        self.assertNotEqual(result.returncode, 0)
        runs = [item["args"] for item in self.commands() if item["args"][0] == "run"]
        self.assertEqual(len(runs), 1, result.stderr)
        self.assertIn("--dry-run=true", runs[0])
        self.assertEqual(len(runs[0][runs[0].index("--default-password") + 1]), 12)

    def test_prebuilt_requires_merged_commit_and_matching_images(self):
        self.env["DOCKER"] = str(self.bin / "docker")
        self.env["GIT"] = self.executable("git", """import os, sys
args = sys.argv[1:]
if args[0] == "merge-base":
    sys.exit(0 if os.environ.get("MERGED") == "1" else 1)
if args[0] == "rev-parse":
    print("a" * 40)
elif "get-url" in args:
    print("https://github.com/example/cedar.git")
""")
        for name, merged, revision, success in [
            ("unmerged", "0", "a" * 40, False),
            ("wrong image", "1", "d" * 40, False),
            ("missing label", "1", "", False),
            ("matched", "1", "a" * 40, True),
        ]:
            with self.subTest(name=name):
                self.log.unlink(missing_ok=True)
                self.env.update(MERGED=merged, IMAGE_REVISION=revision, AGP_GIT_REF="feature")
                result = self.run_script("nas-deploy-prebuilt.sh")
                self.assertEqual(result.returncode == 0, success, result.stderr)
                up = [item for item in self.commands() if "up" in item["args"]]
                self.assertEqual(len(up), int(success))
                if success:
                    self.assertIn("--no-build", up[0]["args"])
                    self.assertIn("--pull", up[0]["args"])
                    self.assertIn("sha256:" + "b" * 64, up[0]["images"])
                    self.assertIn("sha256:" + "c" * 64, up[0]["images"])
                self.assertFalse((self.root / "deploy.lock").exists())


if __name__ == "__main__":
    unittest.main()
