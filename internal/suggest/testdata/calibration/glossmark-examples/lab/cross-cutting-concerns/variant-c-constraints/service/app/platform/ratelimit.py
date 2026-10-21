# @gloss implements .RateLimiting scope=file
"""Per-principal token-bucket rate limiting, configured in ratelimit.yaml."""
from __future__ import annotations

import functools
import threading
import time

import yaml
from flask import Flask, current_app, g, request
from werkzeug.exceptions import TooManyRequests


class TokenBucket:
    def __init__(self, per_minute: int, burst: int) -> None:
        self.rate = per_minute / 60.0
        self.capacity = burst
        self.tokens = float(burst)
        self.updated = time.monotonic()
        self.lock = threading.Lock()

    def take(self) -> bool:
        with self.lock:
            now = time.monotonic()
            self.tokens = min(self.capacity, self.tokens + (now - self.updated) * self.rate)
            self.updated = now
            if self.tokens < 1:
                return False
            self.tokens -= 1
            return True


class RateLimiter:
    def __init__(self, policies: dict) -> None:
        self.policies = policies
        self.buckets: dict[tuple[str, str], TokenBucket] = {}

    def allow(self, policy: str, key: str) -> bool:
        spec = self.policies[policy]
        bucket = self.buckets.setdefault(
            (policy, key), TokenBucket(spec["per_minute"], spec["burst"])
        )
        return bucket.take()


def rate_limit(policy: str):
    def decorator(view):
        @functools.wraps(view)
        def wrapper(*args, **kwargs):
            principal = g.get("principal")
            key = principal.subject if principal else request.remote_addr or "anon"
            if not current_app.extensions["ratelimit"].allow(policy, key):
                raise TooManyRequests(f"rate limit {policy} exceeded")
            return view(*args, **kwargs)

        return wrapper

    return decorator


def init_app(app: Flask, path: str) -> None:
    with open(path, encoding="utf-8") as fh:
        app.extensions["ratelimit"] = RateLimiter(yaml.safe_load(fh)["policies"])
