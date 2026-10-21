# @gloss implements .RequestLogging scope=file
"""Structured JSON logging and one access-log line per request."""
from __future__ import annotations

import logging
import logging.config
import time
import uuid

import yaml
from flask import Flask, g, request

access_log = logging.getLogger("expenses.access")


def configure_logging(path: str) -> None:
    with open(path, encoding="utf-8") as fh:
        logging.config.dictConfig(yaml.safe_load(fh))


def _start_timer() -> None:
    g.request_id = request.headers.get("X-Request-Id") or uuid.uuid4().hex
    g.started = time.perf_counter()


def _log_request(response):
    principal = g.get("principal")
    access_log.info(
        "request",
        extra={
            "request_id": g.get("request_id"),
            "method": request.method,
            "path": request.path,
            "status": response.status_code,
            "subject": principal.subject if principal else None,
            "duration_ms": round((time.perf_counter() - g.get("started", time.perf_counter())) * 1000, 1),
        },
    )
    response.headers["X-Request-Id"] = g.get("request_id", "")
    return response


def init_app(app: Flask) -> None:
    app.before_request(_start_timer)
    app.after_request(_log_request)
