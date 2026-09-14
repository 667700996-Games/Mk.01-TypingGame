"""Lifecycle tests use a tiny fake Go tool; they never rebuild the game."""
import contextlib
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import sys
import tempfile
import time
import unittest
from unittest import mock

import build


FAKE_GO = r'''#!/usr/bin/env python3
import os, pathlib, subprocess, sys, time
mode = os.environ.get("FAKE_MODE", "success")
if sys.argv[1] == "build":
    temp = pathlib.Path(os.environ["GOTMPDIR"])
    assert temp == pathlib.Path(os.environ["TMPDIR"])
    (temp / "intermediate").write_bytes(b"temporary")
    if mode == "wait":
        pathlib.Path("ready").write_text(str(os.getpid()))
        while not pathlib.Path("release").exists():
            time.sleep(0.02)
    if mode == "fail":
        print("intentional failure", flush=True)
        sys.exit(7)
    if mode == "large":
        sys.stdout.buffer.write(b"x" * (2 * 1024 * 1024) + b"FINAL LOG LINE\n")
    pathlib.Path(sys.argv[sys.argv.index("-o") + 1]).write_bytes(b"fake executable")
elif sys.argv[1:3] == ["version", "-m"]:
    print("fake build information")
'''


class LifecycleTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="mk01-lifecycle-")
        self.project = Path(self.temp.name).resolve()
        (self.project / "scripts").mkdir()
        shutil.copyfile(Path(build.__file__), self.project / "scripts" / "build.py")
        for filename in ("README.md", "LICENSE"):
            (self.project / filename).write_text(filename)
        tools = self.project / "tools"
        tools.mkdir()
        (tools / "go").write_text(FAKE_GO)
        (tools / "go").chmod(0o755)
        self.env = dict(os.environ, PATH=str(tools) + os.pathsep + os.environ["PATH"])
        self.root = self.project / ".build"
        self.processes = []

    def tearDown(self):
        (self.project / "release").touch()
        for process in self.processes:
            if process.poll() is None:
                process.terminate()
            try:
                process.communicate(timeout=8)
            except subprocess.TimeoutExpired:
                process.kill()
                process.communicate()
        # Any deliberately orphaned fake tool exits when release appears.
        if self.root.exists():
            for _ in range(100):
                if self.invoke("clean").returncode == 0:
                    break
                time.sleep(0.02)
        self.temp.cleanup()

    def invoke(self, action="build", mode="success"):
        return subprocess.run([sys.executable, "-B", str(self.project / "scripts" / "build.py"), action],
                              env=dict(self.env, FAKE_MODE=mode), capture_output=True, timeout=15)

    def start(self):
        process = subprocess.Popen([sys.executable, "-B", str(self.project / "scripts" / "build.py"), "build"],
                                   env=dict(self.env, FAKE_MODE="wait"), stdout=subprocess.PIPE,
                                   stderr=subprocess.PIPE)
        self.processes.append(process)
        self.until(lambda: (self.project / "ready").exists())
        return process

    def until(self, predicate):
        deadline = time.monotonic() + 8
        while time.monotonic() < deadline:
            if predicate():
                return
            time.sleep(0.02)
        self.fail("Timed out waiting for lifecycle event")

    def assert_empty_work(self):
        self.assertEqual(list((self.root / "work").iterdir()), [])

    def digest_current(self):
        pointer = (self.root / "current.json").read_bytes()
        directory = self.root / "dev" / json.loads(pointer)["id"]
        return pointer, {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in directory.iterdir()}

    def test_success_package_and_retention(self):
        preserved = {}
        for name in ("releases/v1/package", "rollback/base", "symbols/crash", "saves/personal-best",
                     "shared-cache/dependency"):
            path = self.project / name
            path.parent.mkdir(parents=True)
            path.write_bytes(b"preserved original")
            preserved[path] = path.read_bytes()
        for _ in range(12):
            result = self.invoke("package")
            self.assertEqual(result.returncode, 0, result.stderr)
        self.assert_empty_work()
        self.assertEqual(len(list((self.root / "dev").iterdir())), 2)
        self.assertEqual(len(list((self.root / "logs").iterdir())), 10)
        self.assertTrue(self.digest_current()[1])
        self.assertTrue(all(path.read_bytes() == data for path, data in preserved.items()))

    def test_failure_preserves_good_output(self):
        self.assertEqual(self.invoke().returncode, 0)
        before = self.digest_current()
        result = self.invoke(mode="fail")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(before, self.digest_current())
        self.assert_empty_work()

    def test_log_cap_keeps_tail(self):
        self.assertEqual(self.invoke(mode="large").returncode, 0)
        log = next((self.root / "logs").glob("*/output.log")).read_bytes()
        self.assertEqual(len(log), build.LIMIT)
        self.assertIn(b"FINAL LOG LINE", log)

    def test_normal_interrupts(self):
        self.assertEqual(self.invoke().returncode, 0)
        before = self.digest_current()
        for sig in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP):
            with self.subTest(signal=sig):
                (self.project / "ready").unlink(missing_ok=True)
                process = self.start()
                process.send_signal(sig)
                _, err = process.communicate(timeout=8)
                self.assertEqual(process.returncode, 128 + sig, err)
                self.assert_empty_work()
                self.assertEqual(before, self.digest_current())

    def test_concurrent_job_is_protected(self):
        process = self.start()
        temporary = next((self.root / "work").glob("*/tmp/intermediate"))
        result = self.invoke("clean")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(b"Another build/child", result.stderr)
        self.assertTrue(temporary.exists())
        (self.project / "release").touch()
        _, err = process.communicate(timeout=8)
        self.assertEqual(process.returncode, 0, err)
        self.assert_empty_work()

    def test_sigkill_recovery_waits_for_orphan(self):
        self.assertEqual(self.invoke().returncode, 0)
        before = self.digest_current()
        process = self.start()
        process.kill()
        process.wait(timeout=8)
        result = self.invoke("clean")
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue(list((self.root / "work").iterdir()))
        (self.project / "release").touch()
        process.communicate(timeout=8)
        self.until(lambda: self.invoke("clean").returncode == 0)
        self.assert_empty_work()
        self.assertEqual(before, self.digest_current())

    def test_unowned_and_symlink_paths_preserved(self):
        self.assertEqual(self.invoke("clean").returncode, 0)
        foreign = self.root / "work" / ("f" * 32)
        foreign.mkdir()
        (foreign / "source.txt").write_text("keep")
        sentinel = self.project / "release-symbols"
        sentinel.mkdir()
        (sentinel / "symbol").write_text("keep")
        (self.root / "work" / ("e" * 32)).symlink_to(sentinel, target_is_directory=True)
        result = self.invoke("clean")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(str(foreign).encode(), result.stderr)
        self.assertEqual((foreign / "source.txt").read_text(), "keep")
        self.assertEqual((sentinel / "symbol").read_text(), "keep")

    def test_root_symlink_refused(self):
        sentinel = self.project / "precious"
        sentinel.mkdir()
        self.root.symlink_to(sentinel, target_is_directory=True)
        result = self.invoke("clean")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(list(sentinel.iterdir()), [])
        self.root.unlink()

    def test_cleanup_failure_warns_with_path(self):
        self.assertEqual(self.invoke().returncode, 0)
        runner = build.Runner(self.project)
        target = next((self.root / "dev").iterdir())
        with runner.locked(), mock.patch.object(build.shutil, "rmtree", side_effect=OSError("denied")):
            with contextlib.redirect_stderr(__import__("io").StringIO()) as error:
                self.assertFalse(runner.remove(target, "dev"))
            self.assertIn(str(target), error.getvalue())
            self.assertIn("WARNING", error.getvalue())
        self.assertTrue(target.exists())

    def test_failed_publication_keeps_previous_pointer(self):
        self.assertEqual(self.invoke().returncode, 0)
        before = self.digest_current()
        runner = build.Runner(self.project)
        replace = os.replace

        def fail_pointer(source, destination):
            if Path(destination) == self.root / "current.json":
                raise OSError("simulated publication failure")
            return replace(source, destination)

        with mock.patch.dict(os.environ, self.env), mock.patch.object(build.os, "replace", side_effect=fail_pointer):
            self.assertNotEqual(runner.execute("build"), 0)
        self.assertEqual(before, self.digest_current())
        self.assert_empty_work()

    def test_abandoned_stage_and_power_loss_style_recovery(self):
        self.assertEqual(self.invoke().returncode, 0)
        runner = build.Runner(self.project)
        with runner.locked():
            work = self.root / "work" / ("a" * 32)
            work.mkdir()
            build.atomic_json(work / "owner.json", dict(runner.identity, kind="work", id=work.name, pgid=0))
            (work / "stage").mkdir()
            (work / "stage" / "partial").write_text("incomplete package")
        before = self.digest_current()
        self.assertEqual(self.invoke("clean").returncode, 0)
        self.assert_empty_work()
        self.assertEqual(before, self.digest_current())

    def test_live_group_without_inherited_lock_is_preserved(self):
        self.assertEqual(self.invoke("clean").returncode, 0)
        runner = build.Runner(self.project)
        work = self.root / "work" / ("b" * 32)
        with runner.locked():
            work.mkdir()
            build.atomic_json(work / "owner.json", dict(runner.identity, kind="work", id=work.name,
                                                       pgid=os.getpgrp()))
        result = self.invoke("clean")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(b"still exists", result.stderr)
        self.assertTrue(work.exists())
        with runner.locked():
            build.atomic_json(work / "owner.json", dict(runner.identity, kind="work", id=work.name, pgid=0))
        self.assertEqual(self.invoke("clean").returncode, 0)

    def test_child_gate_prevents_work_before_metadata(self):
        runner = build.Runner(self.project)
        write_json = build.atomic_json

        def fail_group_metadata(path, value):
            if value.get("kind") == "work" and value.get("pgid"):
                raise OSError("simulated metadata write failure")
            return write_json(path, value)

        with mock.patch.dict(os.environ, dict(self.env, FAKE_MODE="wait")), \
                mock.patch.object(build, "atomic_json", side_effect=fail_group_metadata):
            self.assertNotEqual(runner.execute("build"), 0)
        self.assertFalse((self.project / "ready").exists())
        self.assert_empty_work()

    def test_owned_tree_symlink_does_not_delete_target(self):
        self.assertEqual(self.invoke("clean").returncode, 0)
        runner = build.Runner(self.project)
        sentinel = self.project / "original-asset"
        sentinel.write_bytes(b"keep original")
        with runner.locked():
            work = self.root / "work" / ("c" * 32)
            work.mkdir()
            build.atomic_json(work / "owner.json", dict(runner.identity, kind="work", id=work.name, pgid=0))
            (work / "link").symlink_to(sentinel)
        self.assertEqual(self.invoke("clean").returncode, 0)
        self.assertEqual(sentinel.read_bytes(), b"keep original")
        self.assert_empty_work()


if __name__ == "__main__":
    unittest.main()
