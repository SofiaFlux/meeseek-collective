#!/usr/bin/env python3
"""Reproducibility spike for Summa42 SQLite persistence semantics.

Uses only Python stdlib sqlite3. This is not production code; it is an
executable architecture experiment for the MVC failure/transaction model.
"""

from __future__ import annotations

import hashlib
import sqlite3
import sys
import tempfile
import time
from pathlib import Path


def check(name: str, condition: bool) -> None:
    if not condition:
        raise AssertionError(name)
    print(f"PASS: {name}")


def intent_hash(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def connect(path: Path) -> sqlite3.Connection:
    con = sqlite3.connect(path)
    con.execute("PRAGMA journal_mode=WAL")
    con.execute("PRAGMA synchronous=FULL")
    con.execute("PRAGMA foreign_keys=ON")
    con.execute("PRAGMA busy_timeout=5000")
    return con


def schema(con: sqlite3.Connection) -> None:
    con.executescript(
        """
        CREATE TABLE policy_state (
            singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
            policy_hash TEXT NOT NULL,
            allow_dispatch INTEGER NOT NULL CHECK (allow_dispatch IN (0,1))
        );

        CREATE TABLE tasks (
            id TEXT PRIMARY KEY,
            current_fence INTEGER NOT NULL,
            current_attempt_id TEXT,
            state TEXT NOT NULL,
            hard_limit INTEGER NOT NULL,
            settled_cost INTEGER NOT NULL DEFAULT 0,
            active_reservations INTEGER NOT NULL DEFAULT 0,
            unresolved_exposure INTEGER NOT NULL DEFAULT 0
        );

        CREATE TABLE attempts (
            id TEXT PRIMARY KEY,
            task_id TEXT NOT NULL REFERENCES tasks(id),
            fence INTEGER NOT NULL,
            lease_state TEXT NOT NULL,
            lease_expires_at INTEGER NOT NULL,
            state TEXT NOT NULL
        );

        CREATE TABLE effect_slots (
            id TEXT PRIMARY KEY,
            task_id TEXT NOT NULL REFERENCES tasks(id),
            slot_key TEXT NOT NULL,
            intent_hash TEXT NOT NULL,
            intent_revision INTEGER NOT NULL DEFAULT 1,
            UNIQUE(task_id, slot_key)
        );

        CREATE TABLE external_operations (
            id TEXT PRIMARY KEY,
            slot_id TEXT NOT NULL REFERENCES effect_slots(id),
            intent_revision INTEGER NOT NULL,
            state TEXT NOT NULL,
            reservation INTEGER NOT NULL,
            policy_hash TEXT NOT NULL,
            dispatcher_id TEXT
        );

        CREATE TABLE completion_records (
            attempt_id TEXT PRIMARY KEY REFERENCES attempts(id),
            manifest_hash TEXT NOT NULL,
            completed_at INTEGER NOT NULL
        );
        """
    )
    con.execute(
        "INSERT INTO policy_state(singleton, policy_hash, allow_dispatch) VALUES (1, 'policy-v1', 1)"
    )
    con.commit()


with tempfile.TemporaryDirectory(prefix="summa42-sqlite-spike-") as td:
    root = Path(td)
    db = root / "state.db"
    now = int(time.time())
    con = connect(db)
    schema(con)

    # Base task/attempt.
    con.execute(
        "INSERT INTO tasks(id,current_fence,current_attempt_id,state,hard_limit) VALUES (?,?,?,?,?)",
        ("t1", 1, "a1", "EXECUTING", 100),
    )
    con.execute(
        "INSERT INTO attempts(id,task_id,fence,lease_state,lease_expires_at,state) VALUES (?,?,?,?,?,?)",
        ("a1", "t1", 1, "ACTIVE", now + 3600, "RUNNING"),
    )
    con.commit()

    # 1. PREPARED + reservation are one atomic unit.
    con.execute("BEGIN IMMEDIATE")
    con.execute(
        "INSERT INTO effect_slots(id,task_id,slot_key,intent_hash) VALUES (?,?,?,?)",
        ("slot-1", "t1", "purchase-primary-test-resource", intent_hash("qty=1;sku=A")),
    )
    con.execute(
        "INSERT INTO external_operations(id,slot_id,intent_revision,state,reservation,policy_hash) "
        "VALUES (?,?,?,?,?,?)",
        ("op-1", "slot-1", 1, "PREPARED", 40, "policy-v1"),
    )
    con.execute("UPDATE tasks SET active_reservations = active_reservations + 40 WHERE id='t1'")
    con.commit()
    op_state = con.execute("SELECT state FROM external_operations WHERE id='op-1'").fetchone()[0]
    reserved = con.execute("SELECT active_reservations FROM tasks WHERE id='t1'").fetchone()[0]
    check("PREPARED operation and reservation commit atomically", op_state == "PREPARED" and reserved == 40)

    # 2. Matching fence alone is insufficient: expired lease rejects authoritative mutation.
    con.execute("UPDATE attempts SET lease_expires_at=? WHERE id='a1'", (now - 1,))
    con.commit()
    cur = con.execute(
        """
        UPDATE tasks
           SET state='AWAITING_VERIFICATION'
         WHERE id='t1'
           AND current_fence=1
           AND current_attempt_id='a1'
           AND state='EXECUTING'
           AND EXISTS (
               SELECT 1 FROM attempts a
                WHERE a.id='a1'
                  AND a.task_id=tasks.id
                  AND a.fence=tasks.current_fence
                  AND a.lease_state='ACTIVE'
                  AND a.lease_expires_at > ?
           )
        """,
        (int(time.time()),),
    )
    con.commit()
    check("expired lease rejects write even when fence still matches", cur.rowcount == 0)
    con.execute("UPDATE attempts SET lease_expires_at=? WHERE id='a1'", (int(time.time()) + 3600,))
    con.commit()

    # 3. Slot identity is stable; parameter/version drift becomes an intent conflict.
    persisted = con.execute(
        "SELECT id,intent_hash,intent_revision FROM effect_slots WHERE task_id=? AND slot_key=?",
        ("t1", "purchase-primary-test-resource"),
    ).fetchone()
    proposed_hash = intent_hash("qty=2;sku=A")
    check("same effect slot resolves to same durable identity", persisted[0] == "slot-1")
    check("changed descriptor is detected as intent conflict", persisted[1] != proposed_hash)

    # 4. Confirmed effect survives Attempt replacement.
    con.execute("UPDATE external_operations SET state='CONFIRMED_EFFECT' WHERE id='op-1'")
    con.execute(
        "INSERT INTO attempts(id,task_id,fence,lease_state,lease_expires_at,state) VALUES (?,?,?,?,?,?)",
        ("a2", "t1", 2, "ACTIVE", int(time.time()) + 3600, "RUNNING"),
    )
    con.execute("UPDATE tasks SET current_fence=2,current_attempt_id='a2' WHERE id='t1'")
    con.commit()
    reused = con.execute(
        """
        SELECT eo.state
          FROM effect_slots es
          JOIN external_operations eo ON eo.slot_id=es.id
         WHERE es.task_id=? AND es.slot_key=?
        """,
        ("t1", "purchase-primary-test-resource"),
    ).fetchone()
    check("confirmed effect is discoverable by replacement Attempt", reused == ("CONFIRMED_EFFECT",))

    # 5. PREPARED is cancelable/not yet committed to dispatch; revocation blocks dispatch.
    con.execute(
        "INSERT INTO tasks(id,current_fence,current_attempt_id,state,hard_limit) VALUES (?,?,?,?,?)",
        ("t2", 1, "a3", "EXECUTING", 100),
    )
    con.execute(
        "INSERT INTO attempts(id,task_id,fence,lease_state,lease_expires_at,state) VALUES (?,?,?,?,?,?)",
        ("a3", "t2", 1, "ACTIVE", int(time.time()) + 3600, "RUNNING"),
    )
    con.execute(
        "INSERT INTO effect_slots(id,task_id,slot_key,intent_hash) VALUES (?,?,?,?)",
        ("slot-2", "t2", "send-notification", intent_hash("recipient=x;body=v1")),
    )
    con.execute(
        "INSERT INTO external_operations(id,slot_id,intent_revision,state,reservation,policy_hash) "
        "VALUES (?,?,?,?,?,?)",
        ("op-2", "slot-2", 1, "PREPARED", 1, "policy-v1"),
    )
    con.execute("UPDATE policy_state SET policy_hash='policy-v2', allow_dispatch=0 WHERE singleton=1")
    con.commit()
    cur = con.execute(
        """
        UPDATE external_operations
           SET state='DISPATCHED', dispatcher_id='dispatcher-1'
         WHERE id='op-2'
           AND state='PREPARED'
           AND policy_hash=(SELECT policy_hash FROM policy_state WHERE singleton=1)
           AND (SELECT allow_dispatch FROM policy_state WHERE singleton=1)=1
        """
    )
    con.commit()
    check("policy/authority change before dispatch prevents PREPARED→DISPATCHED", cur.rowcount == 0)

    # 6. A blob on disk is not completion. Completion becomes authoritative only with
    #    completion record + Attempt/Task transition in one DB transaction.
    con.execute(
        "INSERT INTO tasks(id,current_fence,current_attempt_id,state,hard_limit) VALUES (?,?,?,?,?)",
        ("t3", 1, "a4", "EXECUTING", 100),
    )
    con.execute(
        "INSERT INTO attempts(id,task_id,fence,lease_state,lease_expires_at,state) VALUES (?,?,?,?,?,?)",
        ("a4", "t3", 1, "ACTIVE", int(time.time()) + 3600, "RUNNING"),
    )
    con.commit()
    orphan_blob = root / "sha256-orphan"
    orphan_blob.write_text("executor output", encoding="utf-8")
    con.close()

    con = connect(db)
    no_completion = con.execute("SELECT 1 FROM completion_records WHERE attempt_id='a4'").fetchone()
    task_state = con.execute("SELECT state FROM tasks WHERE id='t3'").fetchone()[0]
    check("orphan/staged output is not mistaken for completed Attempt", no_completion is None and task_state == "EXECUTING")

    con.execute("BEGIN IMMEDIATE")
    con.execute(
        "INSERT INTO completion_records(attempt_id,manifest_hash,completed_at) VALUES (?,?,?)",
        ("a4", intent_hash(orphan_blob.read_text(encoding="utf-8")), int(time.time())),
    )
    con.execute("UPDATE attempts SET state='COMPLETED' WHERE id='a4'")
    con.execute("UPDATE tasks SET state='AWAITING_VERIFICATION' WHERE id='t3'")
    con.commit()
    con.close()

    con = connect(db)
    durable = con.execute(
        """
        SELECT t.state, a.state, c.manifest_hash
          FROM tasks t
          JOIN attempts a ON a.id=t.current_attempt_id
          JOIN completion_records c ON c.attempt_id=a.id
         WHERE t.id='t3'
        """
    ).fetchone()
    check("completion record and AWAITING_VERIFICATION survive reopen", durable is not None and durable[0:2] == ("AWAITING_VERIFICATION", "COMPLETED"))

    # 7. Unresolved exposure participates in available-budget calculation.
    con.execute(
        "INSERT INTO tasks(id,current_fence,current_attempt_id,state,hard_limit,settled_cost,active_reservations,unresolved_exposure) "
        "VALUES (?,?,?,?,?,?,?,?)",
        ("t4", 1, None, "ELIGIBLE", 100, 0, 20, 70),
    )
    hard, settled, reservations, unresolved = con.execute(
        "SELECT hard_limit,settled_cost,active_reservations,unresolved_exposure FROM tasks WHERE id='t4'"
    ).fetchone()
    available = hard - settled - reservations - unresolved
    check("unresolved exposure blocks overcommitting retry", available == 10 and available < 20)

    con.close()

print(f"Runtime: Python {sys.version.split()[0]}, SQLite {sqlite3.sqlite_version}")
