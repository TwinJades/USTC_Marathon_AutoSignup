import hashlib
import json
import msvcrt
import sqlite3
from contextlib import contextmanager
from datetime import datetime, timezone
from pathlib import Path

from .config import RUNTIME, Target
from .errors import Problem


def account_key(username: str) -> str:
    return hashlib.sha256(username.encode("utf-8")).hexdigest()


@contextmanager
def single_instance(root: Path = RUNTIME):
    root.mkdir(parents=True, exist_ok=True)
    with (root / "process.lock").open("a+b") as handle:
        if handle.tell() == 0:
            handle.write(b"0")
            handle.flush()
        handle.seek(0)
        msvcrt.locking(handle.fileno(), msvcrt.LK_NBLCK, 1)
        try:
            yield
        finally:
            handle.seek(0)
            msvcrt.locking(handle.fileno(), msvcrt.LK_UNLCK, 1)


class Journal:
    def __init__(self, root: Path = RUNTIME):
        root.mkdir(parents=True, exist_ok=True)
        self.connection = sqlite3.connect(root / "state.sqlite3")
        self.connection.execute("PRAGMA journal_mode=WAL")
        self.connection.execute("""CREATE TABLE IF NOT EXISTS operations (
            account TEXT NOT NULL, item TEXT NOT NULL, name TEXT NOT NULL,
            campus TEXT NOT NULL, phase TEXT NOT NULL, updated TEXT NOT NULL,
            PRIMARY KEY(account,item))""")
        self.connection.commit()
        self.log_path = root / "activity.jsonl"

    def close(self) -> None:
        self.connection.close()

    def record(self, username: str, target: Target, phase: str) -> None:
        allowed = {"enter_pending", "reserved", "save_pending", "complete", "full", "not_entered"}
        if phase not in allowed:
            raise Problem("未知的报名阶段。")
        self.connection.execute("""INSERT INTO operations VALUES (?,?,?,?,?,?)
            ON CONFLICT(account,item) DO UPDATE SET
            name=excluded.name,campus=excluded.campus,phase=excluded.phase,updated=excluded.updated""",
            (account_key(username), target.id, target.name, target.campus, phase,
             datetime.now(timezone.utc).isoformat()))
        self.connection.commit()
        self.event("报名阶段", item=target.id, phase=phase)

    def rows(self) -> list[dict]:
        self.connection.row_factory = sqlite3.Row
        return [dict(row) for row in self.connection.execute("SELECT * FROM operations ORDER BY updated DESC")]

    def unresolved(self, username: str) -> list[dict]:
        key = account_key(username)
        rows = [row for row in self.rows() if row["phase"] in {"enter_pending", "reserved", "save_pending"}]
        if any(row["account"] != key for row in rows):
            raise Problem("存在其他账户的未完成提交，请切换到对应账户核查后继续。")
        if len(rows) > 1:
            raise Problem("存在多场未完成提交，禁止继续自动报名。")
        return rows

    def event(self, message: str, **fields: str | int | float) -> None:
        payload = {"time": datetime.now(timezone.utc).isoformat(), "event": message, **fields}
        with self.log_path.open("a", encoding="utf-8") as handle:
            handle.write(json.dumps(payload, ensure_ascii=False) + "\n")
