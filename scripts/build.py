#!/usr/bin/env python3
"""Owned, serialized desktop development builds. Python 3.9+, macOS/Linux."""
import argparse
import contextlib
import hashlib
import json
import os
from pathlib import Path
import re
import selectors
import shutil
import signal
import stat
import subprocess
import sys
import tarfile
import time
import uuid

if os.name != "posix":
    raise SystemExit("This runner requires POSIX locks/process groups (macOS or Linux).")
import fcntl

PROJECT = Path(__file__).resolve().parent.parent
LIMIT = 1024 * 1024
TOKEN = re.compile(r"^[0-9a-f]{32}$")
OWNER = "mk01-build-v1"


def atomic_json(path, value):
    temporary = path.with_name(path.name + ".next")
    with open(temporary, "w", encoding="utf-8") as stream:
        json.dump(value, stream, ensure_ascii=False, indent=2)
        stream.flush()
        os.fsync(stream.fileno())
    os.replace(temporary, path)
    sync_directory(path.parent)


def sync_directory(path):
    fd = os.open(path, os.O_RDONLY)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


def regular(path):
    return stat.S_ISREG(path.lstat().st_mode)


def group_alive(pgid):
    if not pgid:
        return False
    try:
        os.killpg(pgid, 0)
        return True
    except ProcessLookupError:
        return False
    except PermissionError:
        return True  # Uncertain ownership/liveness: preserve.


class Interrupted(Exception):
    pass


class Runner:
    def __init__(self, project=PROJECT):
        self.project = Path(project).resolve()
        self.root = self.project / ".build"
        self.identity = {"owner": OWNER, "project": str(self.project)}
        self.warnings = []
        self.interrupted = 0
        self.lock_fd = None

    def warn(self, path, reason):
        message = f"WARNING: {reason}; remaining path: {path}"
        self.warnings.append(message)
        print(message, file=sys.stderr, flush=True)

    def directory(self, path):
        if path.is_symlink() or not path.is_dir() or path.stat().st_uid != os.getuid():
            raise RuntimeError(f"Refusing non-owned directory: {path}")

    def read_json(self, path):
        if not regular(path):
            raise RuntimeError(f"Refusing non-regular metadata: {path}")
        return json.loads(path.read_text(encoding="utf-8"))

    @contextlib.contextmanager
    def locked(self):
        try:
            self.root.mkdir(mode=0o700)
        except FileExistsError:
            self.directory(self.root)
        else:
            atomic_json(self.root / "owner.json", self.identity)
        if self.read_json(self.root / "owner.json") != self.identity:
            raise RuntimeError(f"Unrecognized owner: {self.root}; preserved")
        fd = os.open(self.root / "lock", os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
        try:
            if not stat.S_ISREG(os.fstat(fd).st_mode):
                raise RuntimeError("Non-regular build lock")
            try:
                fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
            except BlockingIOError:
                raise RuntimeError(f"Another build/child owns {self.root}; retry after it exits")
            self.lock_fd = fd
            for name in ("work", "dev", "logs"):
                path = self.root / name
                path.mkdir(exist_ok=True, mode=0o700)
                self.directory(path)
            yield
        finally:
            # Do NOT LOCK_UN: an orphan child may still hold this open-file lock.
            os.close(fd)
            self.lock_fd = None

    def metadata(self, path, kind):
        self.directory(path)
        if not TOKEN.fullmatch(path.name):
            raise RuntimeError("Not a managed run ID")
        value = self.read_json(path / "owner.json")
        if any(value.get(k) != v for k, v in self.identity.items()):
            raise RuntimeError("Owner mismatch")
        if value.get("kind") != kind or value.get("id") != path.name:
            raise RuntimeError("Kind/run ID mismatch")
        return value

    def remove(self, path, kind):
        try:
            if path.parent != self.root / {"work": "work", "dev": "dev", "log": "logs"}[kind]:
                raise RuntimeError("Outside the fixed managed directory")
            self.metadata(path, kind)
            shutil.rmtree(path)  # Only an immediate, owned child; never follows symlinks.
            print(f"Removed {path} ({kind}: stale temporary work or retention)", flush=True)
            return True
        except (OSError, ValueError, RuntimeError) as exc:
            self.warn(path, str(exc))
            return False

    def archive_log(self, work, status):
        target = self.root / "logs" / work.name
        source = work / "log"
        if source.exists():
            self.directory(source)
            atomic_json(source / "owner.json", dict(self.identity, kind="log", id=work.name,
                        completed_ns=time.time_ns(), status=status))
            os.rename(source, target)

    def recover(self):
        for path in sorted((self.root / "work").iterdir()):
            try:
                meta = self.metadata(path, "work")
                pgid = meta.get("pgid", 0)
                if not isinstance(pgid, int) or pgid < 0:
                    raise RuntimeError("Invalid process group metadata")
                if group_alive(pgid):
                    self.warn(path, f"Process group {pgid} still exists; preserved")
                    continue
                self.archive_log(path, "recovered after interruption")
                self.remove(path, "work")
            except (OSError, ValueError, RuntimeError) as exc:
                self.warn(path, str(exc))
        self.prune()

    def current(self):
        path = self.root / "current.json"
        if not path.exists() and not path.is_symlink():
            return None
        value = self.read_json(path)
        if value.get("owner") != OWNER or not TOKEN.fullmatch(value.get("id", "")):
            raise RuntimeError(f"Unknown current artifact pointer: {path}")
        self.metadata(self.root / "dev" / value["id"], "dev")
        return value["id"]

    def prune(self):
        for kind, name, count in (("dev", "dev", 2), ("log", "logs", 10)):
            try:
                current = self.current() if kind == "dev" else None
                entries = []
                for path in (self.root / name).iterdir():
                    try:
                        meta = self.metadata(path, kind)
                        completed = meta["completed_ns"]
                        if not isinstance(completed, int):
                            raise RuntimeError("Invalid completion time")
                        entries.append((completed, path))
                    except (OSError, ValueError, KeyError, RuntimeError) as exc:
                        self.warn(path, str(exc))
                entries.sort(reverse=True)
                keep = {current} if current else set()
                for _, path in entries:
                    if len(keep) < count:
                        keep.add(path.name)
                for _, path in entries:
                    if path.name not in keep:
                        self.remove(path, kind)
            except (OSError, ValueError, RuntimeError) as exc:
                self.warn(self.root / name, str(exc))

    def handle_signal(self, number, frame):
        self.interrupted = number

    def stop(self, process):
        if group_alive(process.pid):
            try:
                os.killpg(process.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
        try:
            process.wait(timeout=3)
        except subprocess.TimeoutExpired:
            pass
        if group_alive(process.pid):
            try:
                os.killpg(process.pid, signal.SIGKILL)
            except ProcessLookupError:
                pass
        process.wait()

    def command(self, args, work, meta, env):
        if self.interrupted:
            raise Interrupted()
        print("+ " + " ".join(map(str, args)), flush=True)
        gate_read, gate_write = os.pipe()
        try:
            process = subprocess.Popen(
                [sys.executable, "-B", str(Path(__file__).resolve()), "_worker", str(gate_read),
                 str(self.lock_fd), *map(str, args)],
                cwd=self.project, env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                start_new_session=True, pass_fds=(self.lock_fd, gate_read))
        except BaseException:
            os.close(gate_write)
            raise
        finally:
            os.close(gate_read)
        log = work / "log" / "output.log"
        tail = bytearray(log.read_bytes() if log.exists() else b"")
        try:
            meta["pgid"] = process.pid
            atomic_json(work / "owner.json", meta)
            os.write(gate_write, b"1")  # No compiler runs before recovery metadata is durable.
            os.close(gate_write)
            gate_write = None
            with selectors.DefaultSelector() as selector:
                selector.register(process.stdout, selectors.EVENT_READ)
                while selector.get_map():
                    if self.interrupted:
                        raise Interrupted()
                    for key, _ in selector.select(timeout=0.2):
                        chunk = os.read(key.fd, 65536)
                        if not chunk:
                            selector.unregister(key.fileobj)
                            continue
                        sys.stdout.buffer.write(chunk)
                        sys.stdout.buffer.flush()
                        tail.extend(chunk)
                        del tail[:-LIMIT]
                        with open(log, "wb") as stream:
                            stream.write(tail)
                result = process.wait()
            if self.interrupted:
                raise Interrupted()
            if result:
                raise RuntimeError(f"Command exited {result}: {args[0]}")
            return bytes(tail)
        finally:
            if gate_write is not None:
                os.close(gate_write)
            if process.poll() is None or group_alive(process.pid):
                self.stop(process)
            process.stdout.close()

    def execute(self, action):
        with self.locked():
            self.recover()
            if action == "clean":
                return 1 if self.warnings else 0
            run_id = uuid.uuid4().hex
            work = self.root / "work" / run_id
            work.mkdir(mode=0o700)
            meta = dict(self.identity, kind="work", id=run_id, pgid=0, action=action)
            atomic_json(work / "owner.json", meta)
            old_handlers = {sig: signal.signal(sig, self.handle_signal)
                            for sig in (signal.SIGINT, signal.SIGTERM, signal.SIGHUP)}
            result = 1
            try:
                (work / "tmp").mkdir()
                (work / "log").mkdir()
                env = dict(os.environ)
                for variable in ("GOTMPDIR", "TMPDIR", "TMP", "TEMP"):
                    env[variable] = str(work / "tmp")
                if action in ("test", "run"):
                    self.command(["go", action, "./..." if action == "test" else "."], work, meta, env)
                else:
                    stage = work / "stage"
                    stage.mkdir()
                    binary = stage / "typing-game"
                    self.command(["go", "build", "-o", str(binary), "."], work, meta, env)
                    if not regular(binary) or binary.stat().st_size == 0:
                        raise RuntimeError("Build did not create a regular non-empty executable")
                    self.command(["go", "version", "-m", str(binary)], work, meta, env)
                    artifact = binary
                    if action == "package":
                        artifact = stage / "typing-game.tar.gz"
                        with tarfile.open(artifact, "w:gz") as archive:
                            for source, name in ((binary, "typing-game"),
                                                 (self.project / "LICENSE", "LICENSE"),
                                                 (self.project / "README.md", "README.md")):
                                archive.add(source, arcname=name, recursive=False)
                        with tarfile.open(artifact, "r:gz") as archive:
                            if archive.getnames() != ["typing-game", "LICENSE", "README.md"]:
                                raise RuntimeError("Unexpected development package contents")
                            with archive.extractfile("typing-game") as stream:
                                if hashlib.sha256(stream.read()).digest() != hashlib.sha256(binary.read_bytes()).digest():
                                    raise RuntimeError("Packaged executable checksum mismatch")
                        binary.unlink()  # Identical bytes are now validated inside the archive.
                    with open(artifact, "rb") as stream:
                        os.fsync(stream.fileno())
                    atomic_json(stage / "owner.json", dict(self.identity, kind="dev", id=run_id,
                                completed_ns=time.time_ns(), action=action, artifact=artifact.name,
                                sha256=hashlib.sha256(artifact.read_bytes()).hexdigest()))
                    if self.interrupted:
                        raise Interrupted()
                    self.current()  # Refuse to overwrite unrecognized existing metadata.
                    destination = self.root / "dev" / run_id
                    os.rename(stage, destination)
                    sync_directory(destination)
                    sync_directory(destination.parent)
                    # Publish only after a complete verified generation exists on the same filesystem.
                    pointer = work / "current.json"
                    atomic_json(pointer, {"owner": OWNER, "id": run_id})
                    os.replace(pointer, self.root / "current.json")
                    sync_directory(self.root)
                    print(f"Published development artifact: {destination / artifact.name}", flush=True)
                result = 0
            except Interrupted:
                result = 128 + (self.interrupted or signal.SIGTERM)
                print(f"Interrupted; stopping this job before cleanup: {work}", file=sys.stderr)
            except (OSError, ValueError, RuntimeError, tarfile.TarError) as exc:
                print(f"ERROR: {exc}", file=sys.stderr)
            finally:
                try:
                    if group_alive(meta["pgid"]):
                        self.warn(work, "Child process group still exists; defer cleanup")
                    else:
                        self.archive_log(work, "success" if result == 0 else f"exit {result}")
                        self.remove(work, "work")
                    self.prune()
                except (OSError, ValueError, RuntimeError) as exc:
                    self.warn(work, str(exc))
                for sig, handler in old_handlers.items():
                    signal.signal(sig, handler)
            return result or (1 if self.warnings else 0)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("build", "package", "test", "run", "clean"))
    args = parser.parse_args()
    runner = Runner()
    try:
        return runner.execute(args.action)
    except (OSError, ValueError, RuntimeError) as exc:
        runner.warn(runner.root, str(exc))
        return 1


if __name__ == "__main__":
    if len(sys.argv) > 1 and sys.argv[1] == "_worker":
        gate, lock = map(int, sys.argv[2:4])
        ready = os.read(gate, 1)
        os.close(gate)
        if ready != b"1":
            sys.exit(1)
        os.set_inheritable(lock, True)
        os.execvpe(sys.argv[4], sys.argv[4:], os.environ)
    sys.exit(main())
